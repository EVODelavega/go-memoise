package memoise

import (
	"fmt"
	"testing"
	"time"
)

func TestCacheScalarValueSet(t *testing.T) {
	data := map[string]interface{}{
		"int":    123,
		"string": "foobar",
		"float":  12.3,
	}
	// create new default cache
	cache := New()
	// set values:
	for k, v := range data {
		err := cache.Value().Set(k, v)
		if err != nil {
			t.Fatalf("unexpected error %+v setting %s to %v", err, k, v)
		}
	}

	for k, v := range data {
		got, err := cache.Value().Get(k)
		if err != nil {
			t.Fatalf("unexpected error %+v getting %s", err, k)
		}
		if got != v {
			t.Fatalf("expected %s to return %v, got %v", k, v, got)
		}
	}

	// check CAS behaviour:
	for k, v := range data {
		if _, err := cache.Value().CAS(k, v); err == nil {
			t.Fatalf("Expected a CAS error setting duplicate %s key", k)
		}
	}
	// valid CAS
	val := "foobar"
	get, err := cache.Value().CAS(val, val)
	if err != nil {
		t.Fatalf("Unexpected error CAS-ing %s: %+v", val, err)
	}
	getS := get.(string)
	if val != getS {
		t.Fatalf("Expected cached value to be %s, saw %s", val, getS)
	}
	// just add Unset for completeness
	for k := range data {
		cache.Value().Unset(k)
		if cache.Value().Has(k) {
			t.Fatalf("Key %s was unset, should not be returning true on Has call", k)
		}
	}
}

func TestCacheSliceSet(t *testing.T) {
	data := []int{1, 2, 3}
	cache := New()
	if err := cache.Value().Set("slice", data); err != nil {
		t.Fatalf("Unexpected error setting slice: %+v", err)
	}
	got, err := cache.Value().Get("slice")
	if err != nil {
		t.Fatalf("Unexpected error getting slice: %+v", err)
	}
	gotSlice, ok := got.([]int)
	if !ok {
		t.Fatal("Could not cast interface to []int")
	}
	if len(gotSlice) != len(data) {
		t.Fatalf("%v does not match %v", gotSlice, data)
	}
}

func TestCacheMapSet(t *testing.T) {
	data := map[string]int{"foo": 1, "bar": 2}
	cache := New()
	if err := cache.Value().Set("map", data); err != nil {
		t.Fatalf("Unexpected error setting map: %+v", err)
	}
	got, err := cache.Value().Get("map")
	if err != nil {
		t.Fatalf("Unexpected error getting map: %+v", err)
	}
	gotMap, ok := got.(map[string]int)
	if !ok {
		t.Fatal("Could not cast interface to map[string]int")
	}
	if len(gotMap) != len(data) {
		t.Fatalf("%v does not match %v", gotMap, data)
	}
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
	cache := New(DefaultTTL(ValueExpiryNever))
	val, err := cache.Set("call", func() (interface{}, error) {
		i := call()
		return i, nil
	})
	if err != nil {
		t.Fatalf("unexpected error setting call key: %+v", err)
	}
	if !cache.Has("call") {
		t.Fatal("call key not in cache?")
	}
	valAfterSet := call()
	if valAfterSet != 1 {
		t.Fatal("Expected callCount to be 1")
	}
	iVal := val.(int)
	if iVal != 0 {
		t.Fatalf("Expected cached value to be 0: %d instead", iVal)
	}
	gotten, err := cache.Get("call")
	if err != nil {
		t.Fatalf("unexpected error getting value: %+v", err)
	}
	giVal := gotten.(int)
	if giVal != iVal {
		t.Fatalf("expected getting %d to equal set value %d", giVal, iVal)
	}
	// no second call has occured
	if valAfterSet != 1 {
		t.Fatal("Expected callCount to be 1")
	}
	if callCount != 2 {
		t.Fatalf("something is wrong with call func, expected 2, got %d", callCount)
	}
	refresh, err := cache.Refresh("call")
	if err != nil {
		t.Fatalf("unexpected error refreshing value: %+v", err)
	}
	rVal := refresh.(int)
	if rVal == iVal {
		t.Fatal("Refresh didn't call function")
	}
	gotten, err = cache.Get("call")
	if err != nil {
		t.Fatalf("unexpected error getting value after refresh: %+v", err)
	}
	giVal = gotten.(int)
	if giVal != rVal {
		t.Fatalf("refresh val != get value after refresh (%d != %d)", rVal, giVal)
	}
	// test unsetting
	cache.Unset("call")
	gotten, err = cache.Get("call")
	if err == nil {
		t.Fatalf("Expected error, instead got %#v", gotten)
	}
	t.Logf("gotten expected error: %+v", err)
}

// Function tests key-level overrides, how expired elements are refreshed
// and whether or not predictable behaviour is observed
func TestExpireRefreshOnAccess(t *testing.T) {
	msCall := func() Call {
		calls := 1
		return func() (interface{}, error) {
			r := calls
			calls++
			return r, nil
		}
	}()
	noExpiry := func() Call {
		calls := 1
		return func() (interface{}, error) {
			r := calls
			calls++
			return r, nil
		}
	}()
	// use overrides
	cache := New(
		DefaultRefreshType(RefreshOnAccess),
		DefaultTTL(ValueExpiryNever),
	)
	never, err := cache.Set("never", noExpiry)
	if err != nil {
		t.Fatalf("unexpected error setting never key: %+v", err)
	}
	ms, err := cache.Set("ms", msCall, SetTTL(time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error setting ms key: %+v", err)
	}
	iNever := never.(int)
	iMS := ms.(int)
	if iNever != iMS {
		t.Fatalf("Expected first calls to be equal (%d != %d)", iNever, iMS)
	}
	// wait for cache to expire
	time.Sleep(time.Millisecond)
	never, _ = cache.Get("never")
	ms, _ = cache.Get("ms")
	iNever = never.(int)
	iMS = ms.(int)
	if iNever == iMS {
		t.Fatalf("Expired cache should not match unexpired value (%d == %d)", iMS, iNever)
	}
	never, err = cache.Refresh("never")
	if err != nil {
		t.Fatalf("error refreshing cache: %+v", err)
	}
	iNever = never.(int)
	if iNever != iMS {
		t.Fatalf("After refresh, both values should match: %d != %d", iNever, iMS)
	}
	// test CAS behaviour with both expired and non-expired values
	time.Sleep(time.Millisecond)
	if _, err := cache.CAS("never", noExpiry); err == nil {
		t.Fatal("CAS call did not result in error when overriding exisiting key")
	}
	if _, err := cache.CAS("ms", msCall, SetTTL(time.Millisecond)); err == nil {
		t.Fatal("CAS call did not result in error when overriding ms key")
	}
}

func TestCheckDuplicates(t *testing.T) {
	cache := New(
		DefaultDuplicateCheck(CheckDuplicate),
	)
	key := "key"
	val := 42
	cb := func() (interface{}, error) {
		return val, nil
	}
	if ival, err := cache.Set(key, cb); err != nil || ival != interface{}(val) {
		t.Fatalf("set returned an error, or value other than %d (returned %#v, %+v)", val, ival, err)
	}
	if err := cache.Value().Set(key, val); err != nil {
		t.Fatalf("value set returned unexpected error: %+v", err)
	}
	if err := cache.Value().Set(key, "new value"); err == nil {
		t.Fatalf("value set returned no error on duplicate set key %s", key)
	}
	ival, err := cache.Set(key, func() (interface{}, error) {
		return "duplicate", nil
	})
	if err == nil {
		t.Fatalf("set didn't return error when setting duplicate %s", key)
	}
	if ival != nil {
		t.Fatalf("expected failed Set call to retunr nil, instead saw %#v", ival)
	}
}

func TestValueRefresh(t *testing.T) {
	cache := New(
		DefaultTTL(time.Millisecond),  // expire values after 1ms
		DefaultRefreshType(NoRefresh), // don't refresh expired values
	)
	const (
		expireKey = "expires"
		neverKey  = "never"
	)
	val := 42
	_ = cache.Value().Set(expireKey, val)
	_ = cache.Value().Set(neverKey, val, SetTTL(ValueExpiryNever))
	time.Sleep(time.Millisecond)
	if rVal, err := cache.Value().Refresh(expireKey); err != nil || rVal != interface{}(val) {
		t.Fatalf("Refreshing expired value %d returned error, or incorrect value (return: %#v, %+v)", val, err, rVal)
	}
	if rVal, err := cache.Value().Get(neverKey); err != nil || rVal != interface{}(val) {
		t.Fatalf("Getting non-expired value %d returned error, or incorrect value (return: %#v, %+v)", val, err, rVal)
	}

	time.Sleep(time.Millisecond)
	rVal, err := cache.Value().Get(expireKey)
	if err == nil {
		t.Fatalf("expected GET on expired key %s to return error", expireKey)
	}
	if rVal != interface{}(val) {
		t.Fatalf("Expected Get to return error + expired value (actual: %#v, %+v)", rVal, err)
	}
	// this is a pointless call, but hey...
	// both Get and refresh should behave identically
	gVal, gerr := cache.Value().Get(neverKey)
	rVal, err = cache.Value().Refresh(neverKey)
	if err != gerr || gVal != rVal {
		t.Fatalf("Expected Get and Refresh to both act identically, instead Get returned (%#v, %+v), and Refresh (%#v, %+v)", gVal, gerr, rVal, err)
	}
	if err != nil {
		// in this case both Get and Refresh failed
		t.Fatalf("Unexpected error in Get/Refresh calls: %+v", err)
	}
}

func TestExpiryTypes(t *testing.T) {
	cache := New(
		DefaultTTL(time.Millisecond),
		DefaultRefreshType(NoRefresh),
	)
	const (
		noRefreshK       = "no-refresh"
		explicitRefreshK = "explicit-refresh"
	)
	val := interface{}(42)
	cb := func() (interface{}, error) {
		return val, nil
	}
	_, _ = cache.Set(noRefreshK, cb)
	_, _ = cache.Set(explicitRefreshK, cb, SetRefreshType(RefreshExplicit))
	time.Sleep(time.Millisecond)
	if _, err := cache.Get(noRefreshK); err != KeyNotFoundErr {
		t.Fatalf("Expected Get(%s) to return %+v, instead got %+v", noRefreshK, KeyNotFoundErr, err)
	}
	if rVal, err := cache.Get(explicitRefreshK); err != ValueExpiredErr || rVal != val {
		t.Fatalf("expected Get(%s) to return (%#v, %+v), got (%#v, %+v)", explicitRefreshK, val, ValueExpiredErr, rVal, err)
	}
}

func TestReturnStaleOnError(t *testing.T) {
	cache := New(DefaultTTL(time.Millisecond))
	val := 1
	callErr := fmt.Errorf("call error")
	cb := func() (interface{}, error) {
		if val < 2 {
			r := interface{}(val)
			val++
			return r, nil
		}
		return nil, callErr
	}
	rVal, err := cache.Set("key", cb, SetCacheType(CacheValueReturnStaleOnError))
	iVal := rVal.(int)
	if iVal != 1 || err != nil {
		t.Fatalf("Expected call to return 1, and no error, instead got %#v, %+v", rVal, err)
	}
	time.Sleep(time.Millisecond)
	// cache should've expired, we have rigged the call to return an error now, too
	rVal, err = cache.Get("key")
	if err != callErr {
		t.Fatalf("Expected Get to return %+v, instead got %+v", callErr, err)
	}
	iVal = rVal.(int)
	if iVal != 1 {
		t.Fatalf("Expected Get to retunr stale value")
	}
}

func TestReturnErrorNotCached(t *testing.T) {
	cache := New(
		DefaultTTL(time.Millisecond),
		DefaultCacheType(CacheValueReturnError),
	)
	val := 0
	callErr := fmt.Errorf("call error")
	cb := func() (interface{}, error) {
		val++
		if (val % 2) == 1 {
			r := interface{}(val)
			return r, nil
		}
		return nil, callErr
	}
	rVal, err := cache.Set("key", cb, SetCacheType(CacheValueReturnStaleOnError))
	if rVal != interface{}(val) {
		t.Fatalf("Expected call to return 1, and no error, instead got %#v, %+v", rVal, err)
	}
	time.Sleep(time.Millisecond)
	// cache should've expired, we have rigged the call to return an error now, too
	rVal, err = cache.Get("key")
	iVal := rVal.(int) // old value should be 1
	if err != callErr || iVal != 1 {
		t.Fatalf("Expected Get to return (nil, %+v), instead got (%#v, %+v)", callErr, rVal, err)
	}
	rVal, err = cache.Get("key")
	if err != nil || rVal != interface{}(val) {
		t.Fatalf("Expected Get to return (%d, nil), got (%#v, %+v)", val, rVal, err)
	}
}
