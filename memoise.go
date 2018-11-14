package memoise

import (
	"sync"
	"time"
)

type citem struct {
	val     interface{}
	err     error
	expires time.Time
}

type centry struct {
	item *citem
	mu   *sync.RWMutex // mutex at entry level -> used to refresh cache
	cb   Call
	ct   CacheType
	rt   RefreshType
	ttl  time.Duration
}

type vcentry struct {
	item *citem
	mu   *sync.RWMutex
	ttl  time.Duration
}

type cache struct {
	mu              *sync.RWMutex      // duh, we need mutex because... see below
	entries         map[string]*centry // let's not use sync.Map, it's crap anyway
	vCache          *valCache
	defaultCT       CacheType
	defaultRT       RefreshType
	defaultTTL      time.Duration
	checkDuplicates DuplicateCheck
}

// cache for values
type valCache struct {
	mu              *sync.RWMutex
	entries         map[string]*vcentry
	defaultTTL      time.Duration
	checkDuplicates DuplicateCheck
}

// default cache setup
func newCache() *cache {
	return &cache{
		mu:              &sync.RWMutex{},
		entries:         map[string]*centry{},
		defaultCT:       CacheValueReturnError,
		defaultRT:       RefreshOnAccess,
		defaultTTL:      ValueExpiryDefault,
		checkDuplicates: NoDuplicateCheck,
		vCache: &valCache{
			mu:              &sync.RWMutex{},
			entries:         map[string]*vcentry{},
			defaultTTL:      ValueExpiryDefault,
			checkDuplicates: NoDuplicateCheck,
		},
	}
}

func (c *cache) newEntry(cb Call) *centry {
	return &centry{
		item: &citem{
			expires: time.Time{},
		},
		mu:  &sync.RWMutex{},
		cb:  cb,
		ct:  c.defaultCT,
		rt:  c.defaultRT,
		ttl: c.defaultTTL,
	}
}

func (e *centry) initItem() {
	var exp time.Time
	if e.ttl == ValueExpiryNever {
		exp = time.Time{}
	} else {
		exp = time.Now().Add(e.ttl)
	}
	v, err := e.cb()
	e.item = &citem{
		val:     v,
		err:     err,
		expires: exp,
	}
}

// Set - implementation of interface, set a value and return the result of the cached call
func (c *cache) Set(key string, call Call, opts ...EntryConfig) (interface{}, error) {
	if c.checkDuplicates == CheckDuplicate {
		return c.setWithCheck(key, call, opts...)
	}
	// Obtain read lock, even though we're writing. This lock is not really doing anything
	// but is here to defend against race conditions in case CAS is called with the same key
	// setWithCheck obtains full lock, RLock allows for reads, still, while a set will be atomic
	c.mu.Lock()
	i, err := c.set(key, call, opts...)
	c.mu.Unlock()
	return i, err
}

func (c *cache) Unset(key string) {
	c.mu.Lock()
	// delete - it's a no-op if the element isn't set, no need to check
	delete(c.entries, key)
	c.mu.Unlock()
}

// CAS - Check & Set, same as set but "atomic", returns DuplicateEntryErr if value already exists
func (c *cache) CAS(key string, call Call, opts ...EntryConfig) (interface{}, error) {
	return c.setWithCheck(key, call, opts...)
}

// Has - check whether or not key is set
func (c *cache) Has(key string) bool {
	c.mu.RLock()
	_, ok := c.entries[key]
	c.mu.RUnlock()
	return ok
}

// Refresh - manually/forcibly refresh given cache value
func (c *cache) Refresh(k string) (interface{}, error) {
	c.mu.RLock()
	ce, err := c.get(k)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	ce.mu.Lock()
	v, err := ce.cb()
	if err == nil || ce.ct == CacheAll {
		ce.item.val = v
		ce.item.err = err
		if ce.ttl != ValueExpiryNever {
			ce.item.expires = time.Now().Add(ce.ttl)
		}
		ce.mu.Unlock()
		return v, err
	}
	if ce.ct == CacheValueReturnStaleOnError {
		// get stale value
		v = ce.item.val
		ce.mu.Unlock()
		// return stale value + new error
		return v, err
	}
	// default, on error don't update
	ce.mu.Unlock()
	return v, err
}

func (c *cache) autoRefresh(key string, val interface{}, err error) {
	ce := c.entries[key] // this is guaranteed to work -> the janitor calls this, the key exists
	if err == nil || ce.ct == CacheAll {
		ce.item.val = val
		ce.item.err = err
		ce.item.expires = time.Now().Add(ce.ttl)
	}
	// all others should be a no-op, they cannot be auto-refreshed
}

// Get - get cached values
func (c *cache) Get(key string) (interface{}, error) {
	c.mu.RLock()
	ce, err := c.get(key)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	ce.mu.RLock()
	v, err, exp := ce.item.val, ce.item.err, ce.item.expires
	// value is still valid, return and be done with it
	now := time.Now()
	if exp.IsZero() || exp.After(now) {
		ce.mu.RUnlock()
		// this is really optimistic, we're not handling errors correctly ATM
		return v, err
	}
	// value has expired
	if ce.rt == RefreshExplicit {
		ce.mu.RUnlock()
		return v, ErrValueExpired
	}
	if exp.Before(now) && ce.rt == NoRefresh {
		ce.mu.RUnlock()
		c.Unset(key)
		// this entry is gone now
		return nil, ErrKeyNotFound
	}
	ce.mu.RUnlock()
	// ignore RefreshAsync for the time being
	return c.Refresh(key)
}

func (c *cache) Value() ValueCache {
	return c.vCache
}

func (c *cache) setWithCheck(k string, cb Call, opts ...EntryConfig) (interface{}, error) {
	c.mu.Lock()
	if _, ok := c.entries[k]; ok {
		c.mu.Unlock()
		return nil, ErrDuplicateEntry
	}
	// regular call to set, but we have obtained a lock here...
	i, err := c.set(k, cb, opts...)
	c.mu.Unlock()
	return i, err
}

func (c *cache) set(k string, cb Call, opts ...EntryConfig) (interface{}, error) {
	ent := c.newEntry(cb)
	for _, o := range opts {
		o(ent)
	}
	ent.initItem()
	// No error, or we want to cache errors
	if ent.item.err == nil || ent.ct == CacheAll {
		c.entries[k] = ent
		return ent.item.val, ent.item.err
	}
	// ensure expired entry is stored, so next time we don't return cached error
	ent.item.expires = time.Now().Add(-1 * time.Second)
	c.entries[k] = ent
	// return call as it happened
	return ent.item.val, ent.item.err
}

// get, return RAW POINTER of cached value, careful when manipulating this one (use locks!)
func (c *cache) get(k string) (*centry, error) {
	e, ok := c.entries[k]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return e, nil
}

// value cache implementation:

func (c *valCache) Get(key string) (interface{}, error) {
	c.mu.RLock()
	e, err := c.get(key)
	if err != nil {
		if e != nil {
			c.mu.RUnlock()
			return e.item.val, err
		}
		c.mu.RUnlock()
		return nil, err
	}
	// get the cached value
	ret := e.item.val
	c.mu.RUnlock()
	return ret, nil
}

func (c *valCache) Set(key string, value interface{}, opts ...EntryConfig) error {
	c.mu.Lock()
	if c.checkDuplicates == CheckDuplicate {
		if _, ok := c.entries[key]; ok {
			c.mu.Unlock()
			return ErrDuplicateEntry
		}
	}
	c.set(key, value, opts...)
	c.mu.Unlock()
	return nil
}

func (c *valCache) Refresh(key string) (interface{}, error) {
	c.mu.RLock()
	e, err := c.get(key)
	c.mu.RUnlock()
	if err != nil && err != ErrValueExpired {
		return nil, err
	}
	e.mu.Lock()
	ret := e.item.val
	// this is a pointless call
	if e.ttl == ValueExpiryNever {
		e.mu.Unlock()
		return ret, nil
	}
	// only set TTL if we have to
	e.item.expires = time.Now().Add(e.ttl)
	e.mu.Unlock()
	return ret, nil
}

func (c *valCache) Has(key string) bool {
	c.mu.RLock()
	_, ok := c.entries[key]
	c.mu.RUnlock()
	return ok
}

func (c *valCache) CAS(key string, value interface{}, opts ...EntryConfig) (interface{}, error) {
	c.mu.Lock()
	if e, err := c.get(key); err == nil {
		// we have a duplicate
		// return existing entry + error
		c.mu.Unlock()
		return e.item.val, ErrDuplicateEntry
	}
	delete(c.entries, key)
	c.set(key, value, opts...)
	c.mu.Unlock()
	return value, nil
}

func (c *valCache) Unset(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *valCache) get(key string) (*vcentry, error) {
	e, ok := c.entries[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	if !e.item.expires.IsZero() && e.item.expires.Before(time.Now()) {
		return e, ErrValueExpired
	}
	return e, nil
}

func (c *valCache) set(key string, value interface{}, opts ...EntryConfig) {
	// create entry
	e := &vcentry{
		item: &citem{
			val:     value,
			expires: time.Time{},
		},
		mu:  &sync.RWMutex{},
		ttl: c.defaultTTL,
	}
	// configure
	for _, o := range opts {
		o(e)
	}
	// set TTL if value has expiry
	if e.ttl != ValueExpiryNever {
		e.item.expires = time.Now().Add(e.ttl)
	}
	c.entries[key] = e
}

// entryConfigInterface

func (e *centry) setCT(ct CacheType) {
	e.ct = ct
}

func (e *centry) setTTL(ttl time.Duration) {
	e.ttl = ttl
}

func (e *centry) SetRefreshType(rt RefreshType) {
	e.rt = rt
}

func (v *vcentry) setCT(_ CacheType) {}

func (v *vcentry) SetRefreshType(_ RefreshType) {}

func (v *vcentry) setTTL(ttl time.Duration) {
	v.ttl = ttl
}
