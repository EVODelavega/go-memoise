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

type cache struct {
	mu              *sync.RWMutex      // duh, we need mutex because... see below
	entries         map[string]*centry // let's not use sync.Map, it's crap anyway
	defaultCT       CacheType
	defaultRT       RefreshType
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
	}
}

func (c *cache) newEntry(cb Call) *centry {
	return &centry{
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
	c.mu.RLock()
	i, err := c.set(key, call, opts...)
	c.mu.RUnlock()
	return i, err
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
	if err != nil {
		c.mu.RUnlock()
		return nil, err
	}
	ce.mu.Lock()
	v, err := ce.cb()
	if err == nil || ce.ct == CacheAll {
		ce.item.val = v
		ce.item.err = err
		ce.item.expires = time.Now().Add(ce.ttl)
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

// Get - get cached values
func (c *cache) Get(key string) (interface{}, error) {
	c.mu.RLock()
	ce, err := c.get(key)
	if err != nil {
		c.mu.RUnlock()
		return nil, err
	}
	ce.mu.RLock()
	v, err, exp := ce.item.val, ce.item.err, ce.item.expires
	ce.mu.RUnlock()
	// value is still valid, return and be done with it
	if exp.IsZero() || exp.After(time.Now()) {
		// this is really optimistic, we're not handling errors correctly ATM
		return v, err
	}
	// value has expired
	if ce.rt == RefreshExplicit {
		return v, ValueExpiredErr
	}
	// ignore RefreshAsync for the time being
	return c.Refresh(key)
}

func (c *cache) setWithCheck(k string, cb Call, opts ...EntryConfig) (interface{}, error) {
	c.mu.Lock()
	if _, ok := c.entries[k]; ok {
		c.mu.Unlock()
		return nil, DuplicateEntryErr
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
		return nil, KeyNotFoundErr
	}
	return e, nil
}
