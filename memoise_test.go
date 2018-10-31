package memoise

import (
	"testing"
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
