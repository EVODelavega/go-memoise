package memoise

import (
	"errors"
	"time"
)

var (
	// ValueExpiredErr - error returned when RefreshExplicit is set
	ValueExpiredErr   = errors.New("the cache value has expired")
	DuplicateEntryErr = errors.New("cache already contains key")
	KeyNotFoundErr    = errors.New("cache does not contain given key")
)

const (
	// CacheValueReturnError - Default, errors not cached but should call return error,
	// the error is returned to the caller
	CacheValueReturnError CacheType = iota
	// CacheAll - Whether a call returns error or not, cache the result all the same
	CacheAll
	// CacheValueReturnStaleOnError - Cache value, if the call results in error, return error + stale value
	CacheValueReturnStaleOnError
)

const (
	// RefreshOnAccess - should the cached values be expired, refresh ad-hoc
	RefreshOnAccess RefreshType = iota
	// RefreshAsync - do not attempt refresh on access, but leave it to the cache manager
	// getting values from cache might result in blocking calls as a result, use with caution
	RefreshAsync
	// RefreshExplicit - Never refresh automatically, stale values are returned along with ValueExpiredErr
	// error. Cache will not be refreshed until an explicit refresh call is made. This call is blocking
	// both for caller and any concurrent reads!
	RefreshExplicit
)

const (
	// ValueExpiryNever - Convenience, const for values that never expire (can be refreshed explicitly)
	ValueExpiryNever time.Duration = 0
	// ValueExpiryDefault - Default TTL for values is one minute
	ValueExpiryDefault = time.Minute
)

const (
	// NoDuplicateCheck - Do not check duplicate keys (default)
	NoDuplicateCheck DuplicateCheck = iota
	// CheckDuplicate - Before setting new cache value, check if key is duplicate
	CheckDuplicate
)

// Call - function yielding return value + error, these values will be the ones cached
type Call func() (interface{}, error)

// EntryConfig - Type to override default config for a specific entry (used as varargs in Set func)
type EntryConfig func(*centry)

// CacheConf - Variadic arg to set default config for cache
type CacheConf func(*cache)

// CacheType - config values for caching behaviours
type CacheType int

// RefreshType - config values specifying the way values should be refreshed
type RefreshType int

// DuplicateCheck - check if given cache entry already exists before setting (check not performed by default)
type DuplicateCheck int

// Cache - exposed interface of the package
type Cache interface {
	// Set - Add new entry to cachj
	Set(key string, call Call, opts ...EntryConfig) (interface{}, error)
	// Refresh - Explicit refresh for given value (blocking)
	Refresh(key string) (interface{}, error)
	// Has - check if given key exist
	Has(key string) bool
	// CAS - Check And Set, check for duplicate prior to setting value
	CAS(key string, call Call, opts ...EntryConfig) (interface{}, error)
	// Get - Get cached values
	Get(key string) (interface{}, error)
}

// DefaultTTL - Set cache-level default TTL
func DefaultTTL(ttl time.Duration) CacheConf {
	return func(c *cache) {
		c.defaultTTL = ttl
	}
}

// DefaultDuplicateCheck - Set default behaviours on calling "Set"
func DefaultDuplicateCheck(dc DuplicateCheck) CacheConf {
	return func(c *cache) {
		c.checkDuplicates = dc
	}
}

// DefaultCacheType - Set the default cache type for cache
func DefaultCacheType(ct CacheType) CacheConf {
	return func(c *cache) {
		c.defaultCT = ct
	}
}

// DefaultRefreshType - Set cache default refresh behaviour
func DefaultRefreshType(rt RefreshType) CacheConf {
	return func(c *cache) {
		c.defaultRT = rt
	}
}

// SetCacheType - Override cache-type on entry level
func SetCacheType(ct CacheType) EntryConfig {
	return func(e *centry) {
		e.ct = ct
	}
}

// SetRefreshType - Override refresh type on entry level
func SetRefreshType(rt RefreshType) EntryConfig {
	return func(e *centry) {
		e.rt = rt
	}
}

// SetTTL - override TTL on entry level
func SetTTL(ttl time.Duration) EntryConfig {
	return func(e *centry) {
		e.ttl = ttl
	}
}

// New - get new cache object
func New(opts ...CacheConf) Cache {
	c := newCache()
	for _, o := range opts {
		o(c)
	}
	return c
}
