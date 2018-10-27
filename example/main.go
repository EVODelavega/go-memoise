package main

import (
	"fmt"
	"time"

	"github.com/EVODelavega/go-memoise"
)

func main() {
	cache := memoise.New(
		memoise.DefaultTTL(memoise.ValueExpiryNever),
		memoise.DefaultDuplicateCheck(memoise.CheckDuplicate),
	)
	// add callback item:
	cache.Set("slowCall", func() (interface{}, error) {
		i := slowCall()
		return i, nil
	})
	cache.Value().Set("simpleVal", 42)
	fmt.Println("Get value from callback cache multiple times")
	for i := 0; i < 3; i++ {
		v, err := cache.Get("slowCall")
		fmt.Printf("Got %v - err: %v\n", v, err)
	}
	fmt.Println("Get value, refreshing the value each time")
	for i := 0; i < 3; i++ {
		v, err := cache.Refresh("slowCall")
		fmt.Printf("Got %v - err: %+v\n", v, err)
	}
	fmt.Println("getting value from K-V cache:")
	for i := 0; i < 3; i++ {
		v, err := cache.Value().Get("simpleVal")
		fmt.Printf("Got %v - err: %v\n", v, err)
	}
}

func slowCall() int {
	time.Sleep(time.Second * 1)
	return 42
}
