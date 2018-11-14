package memoise_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/EVODelavega/go-memoise"
	"github.com/stretchr/testify/assert"
)

func TestCacheScalarValueSet(t *testing.T) {
	data := map[string]interface{}{
		"int":    123,
		"string": "foobar",
		"float":  12.3,
	}
	// create new default cache
	cache := memoise.New()
	// set values:
	for k, v := range data {
		assert.NoError(t, cache.Value().Set(k, v))
	}

	for k, v := range data {
		got, err := cache.Value().Get(k)
		assert.NoError(t, err)
		assert.Equal(t, v, got)
	}

	// check CAS behaviour:
	for k, v := range data {
		_, err := cache.Value().CAS(k, v)
		assert.Error(t, err)
		assert.Equal(t, memoise.ErrDuplicateEntry, err)
	}
	// valid CAS
	val := "foobar"
	get, err := cache.Value().CAS(val, val)
	assert.NoError(t, err)
	assert.Equal(t, val, get)
	// just add Unset for completeness
	for k := range data {
		cache.Value().Unset(k)
		assert.False(t, cache.Value().Has(k))
	}
	// get a non-existent key
	nVal, err := cache.Value().Get("this key doesn't exist")
	assert.Nil(t, nVal)
	assert.Equal(t, memoise.ErrKeyNotFound, err)
	// refresh non-existent key
	nVal, err = cache.Value().Refresh("this key doesn't exist")
	assert.Nil(t, nVal)
}

func TestCacheSliceSet(t *testing.T) {
	data := []int{1, 2, 3}
	cache := memoise.New()
	assert.NoError(t, cache.Value().Set("slice", data))
	got, err := cache.Value().Get("slice")
	assert.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestCacheMapSet(t *testing.T) {
	data := map[string]int{"foo": 1, "bar": 2}
	cache := memoise.New()
	assert.NoError(t, cache.Value().Set("map", data))
	got, err := cache.Value().Get("map")
	assert.NoError(t, err)
	assert.Equal(t, data, got)
}

// Simple callback-based Set, Get, Unset, Refresh tests
func TestCallSimple(t *testing.T) {
	callCount := 0
	call := func() int {
		r := callCount
		callCount++
		return r
	}
	// cache without expiry
	cache := memoise.New(memoise.DefaultTTL(memoise.ValueExpiryNever))
	val, err := cache.Set("call", func() (interface{}, error) {
		i := call()
		return i, nil
	})
	assert.NoError(t, err)
	assert.True(t, cache.Has("call"))
	valAfterSet := call()
	assert.Equal(t, valAfterSet, 1)
	assert.Equal(t, val, 0)
	gotten, err := cache.Get("call")
	assert.NoError(t, err)
	// func should be called twice
	assert.Equal(t, 2, callCount)
	refresh, err := cache.Refresh("call")
	assert.NoError(t, err)
	assert.NotEqual(t, refresh, val)
	gotten, err = cache.Get("call")
	assert.NoError(t, err)
	assert.Equal(t, refresh, gotten)
	// test unsetting
	cache.Unset("call")
	gotten, err = cache.Get("call")
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrKeyNotFound, err)
}

// Function tests key-level overrides, how expired elements are refreshed
// and whether or not predictable behaviour is observed
func TestExpireRefreshOnAccess(t *testing.T) {
	msCall := func() memoise.Call {
		calls := 1
		return func() (interface{}, error) {
			r := calls
			calls++
			return r, nil
		}
	}()
	noExpiry := func() memoise.Call {
		calls := 1
		return func() (interface{}, error) {
			r := calls
			calls++
			return r, nil
		}
	}()
	// use overrides
	cache := memoise.New(
		memoise.DefaultRefreshType(memoise.RefreshOnAccess),
		memoise.DefaultTTL(memoise.ValueExpiryNever),
	)
	never, err := cache.Set("never", noExpiry)
	assert.NoError(t, err)
	ms, err := cache.Set("ms", msCall, memoise.SetTTL(time.Millisecond))
	assert.NoError(t, err)
	assert.Equal(t, ms, never)
	// wait for cache to expire
	time.Sleep(time.Millisecond)
	never, _ = cache.Get("never")
	ms, _ = cache.Get("ms")
	assert.NotEqual(t, ms, never)
	never, err = cache.Refresh("never")
	assert.NoError(t, err)
	assert.Equal(t, never, ms)
	// test CAS behaviour with both expired and non-expired values
	time.Sleep(time.Millisecond)
	_, err = cache.CAS("never", noExpiry)
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrDuplicateEntry, err)
	_, err = cache.CAS("ms", msCall, memoise.SetTTL(time.Millisecond))
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrDuplicateEntry, err)
}

func TestCheckDuplicates(t *testing.T) {
	cache := memoise.New(
		memoise.DefaultDuplicateCheck(memoise.CheckDuplicate),
	)
	key := "key"
	val := 42
	cb := func() (interface{}, error) {
		return val, nil
	}
	sval, err := cache.Set(key, cb)
	assert.NoError(t, err)
	assert.Equal(t, val, sval)
	assert.NoError(t, cache.Value().Set(key, val))
	err = cache.Value().Set(key, "new value")
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrDuplicateEntry, err)
	seval, err := cache.Set(key, func() (interface{}, error) {
		return "duplicate", nil
	})
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrDuplicateEntry, err)
	assert.Nil(t, seval)
}

func TestValueRefresh(t *testing.T) {
	cache := memoise.New(
		memoise.DefaultTTL(time.Millisecond),          // expire values after 1ms
		memoise.DefaultRefreshType(memoise.NoRefresh), // don't refresh expired values
	)
	const (
		expireKey = "expires"
		neverKey  = "never"
	)
	val := 42
	assert.NoError(t, cache.Value().Set(expireKey, val))
	assert.NoError(t, cache.Value().Set(neverKey, val, memoise.SetTTL(memoise.ValueExpiryNever)))
	time.Sleep(time.Millisecond)
	rVal, err := cache.Value().Refresh(expireKey)
	assert.NoError(t, err)
	assert.Equal(t, val, rVal)
	rVal, err = cache.Value().Get(neverKey)
	assert.NoError(t, err)
	assert.Equal(t, val, rVal)

	time.Sleep(time.Millisecond)
	rVal, err = cache.Value().Get(expireKey)
	assert.Error(t, err)
	assert.Equal(t, memoise.ErrValueExpired, err)
	assert.Equal(t, val, rVal)
	// this is a pointless call, but hey...
	// both Get and refresh should behave identically
	gVal, gerr := cache.Value().Get(neverKey)
	rVal, err = cache.Value().Refresh(neverKey)
	// all equal (no error, all values == val)
	assert.Equal(t, gerr, err)
	assert.Equal(t, gVal, rVal)
	assert.NoError(t, err)
	assert.Equal(t, val, gVal)
}

func TestExpiryTypes(t *testing.T) {
	cache := memoise.New(
		memoise.DefaultTTL(time.Millisecond),
		memoise.DefaultRefreshType(memoise.NoRefresh),
	)
	const (
		noRefreshK       = "no-refresh"
		explicitRefreshK = "explicit-refresh"
	)
	val := 42
	cb := func() (interface{}, error) {
		return val, nil
	}
	nrVal, err := cache.Set(noRefreshK, cb)
	assert.NoError(t, err)
	assert.Equal(t, val, nrVal)
	erVal, err := cache.Set(explicitRefreshK, cb, memoise.SetRefreshType(memoise.RefreshExplicit))
	assert.NoError(t, err)
	assert.Equal(t, val, erVal)
	time.Sleep(time.Millisecond)
	_, err = cache.Get(noRefreshK)
	assert.Equal(t, memoise.ErrKeyNotFound, err)
	rVal, err := cache.Get(explicitRefreshK)
	assert.Equal(t, val, rVal)
	assert.Equal(t, memoise.ErrValueExpired, err)
}

func TestReturnStaleOnError(t *testing.T) {
	cache := memoise.New(memoise.DefaultTTL(time.Millisecond))
	val := 1
	callErr := fmt.Errorf("call error")
	cb := func() (interface{}, error) {
		if val < 2 {
			r := val
			val++
			return r, nil
		}
		return nil, callErr
	}
	rVal, err := cache.Set("key", cb, memoise.SetCacheType(memoise.CacheValueReturnStaleOnError))
	assert.NoError(t, err)
	// val was incremented when func was called
	assert.Equal(t, val-1, rVal)
	time.Sleep(time.Millisecond)
	// cache should've expired, we have rigged the call to return an error now, too
	rVal, err = cache.Get("key")
	assert.Equal(t, err, callErr)
	assert.Equal(t, val-1, rVal)
}

func TestReturnErrorNotCached(t *testing.T) {
	cache := memoise.New(
		memoise.DefaultTTL(time.Millisecond),
		memoise.DefaultCacheType(memoise.CacheValueReturnError),
	)
	val := 0
	callErr := fmt.Errorf("call error")
	cb := func() (interface{}, error) {
		val++
		if (val % 2) == 1 {
			r := val
			return r, nil
		}
		return nil, callErr
	}
	rVal, err := cache.Set("key", cb, memoise.SetCacheType(memoise.CacheValueReturnStaleOnError))
	assert.NoError(t, err)
	assert.Equal(t, val, rVal)
	time.Sleep(time.Millisecond)
	// cache should've expired, we have rigged the call to return an error now, too
	rVal, err = cache.Get("key")
	assert.Equal(t, err, callErr)
	// we should get stale value (prior to increment)
	assert.Equal(t, val-1, rVal)
	// no error, return new value
	rVal, err = cache.Get("key")
	assert.NoError(t, err)
	assert.Equal(t, val, rVal)
}
