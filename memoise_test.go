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
