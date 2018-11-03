package memoise

import (
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
