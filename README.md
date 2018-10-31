# go-memoise
Memoisation/caching golang package

## What is it for/how to use

Sometimes you want to be able to cut down on the number of calls you make to external services, or you want to hold on to the last valid response of an API call. This is what this package does.

It allows you to cache responses, whatever the type may be, and configure how often you want the call to actually happen. Say you need to check whether or not a feature flag is enabled. If you get several hundred calls per second, that's a lot of redundant calls (most likely). Simply wrapping that call in this cache would allow you to restrict the number of actual calls to 1 per second:

```go
cache := memoise.New() // defaults
// add to cache:
cached, err := cache.Set("featureX", func() (interface{}, error) {
    // do whatever here, e.g.:
    return client.IsFeatureEnabled(request)
},
memoise.SetTTL(time.Second * 1),
memoise.SetRefreshType(memoise.RefreshOnAccess), // refresh value if expired, on access
)
enabled := cached.(bool) // say it returns a bool
log.Printf("featureX is currently enabled: %v - call error: %+v", enabled, err)

// where needed:

cached, err := cache.Get("featureX") // after 1 second, the call will be made
enabled := cached.(bool)
if enabled {
    // do featureX
}
```

Of course, you might want to keep certain values in memory _without_ having to provide a callback. Though technically you _could_ write:

```go
cache.Set("value", func() (interface{}, error) {
    return 123, nil
})
```

This does look rather messy, and needlessly complex. The cache object therefore has a simple K-V cache embedded within it. The API is pretty similar, but allows you to pass in a value rather than a callback (and is a bit simpler):

```go
cache.Value().Set("value", 123)
```

## TODO's

Work is currently focussed on the janitor. A component that runs concurrently, periodically checking the values in the cache, and either removing expired values, or refreshing them (depending on how they're configured). This will require a couple of changes. Chief among which: passing in a context (cancelable, ideally), and additional config functions for varargs to specify how the janitor is to interact with the cached values.

Tests are pretty much absent ATM, but work has started adding test cases for various scenario's.

## Contributing

Sure, if you feel like you want/can contribute: feel free to clone and open PR's. Discussion in PR's is encouraged, provided the code is discussed, and nothing else. We all want to write good code, but we all make mistakes.
Politics are not welcome. I don't care if you're a libertarian, communist, or a member of the Monster party. If the code is good, I'm happy to merge it in. Should the code not be up to scratch, people are free to say why they'd rather not see the changes land. That's it.

Criticism is healthy if the goal is to improve the overall quality of the code. That's what contributors and reviewers should keep in mind at all time.

TL;DR: The CoC boils down to this:

***Code is a-political, good code speaks for itself. It doesn't matter who you are, good code is welcome, bad code will be criticised. Criticism should aim to improve the code, so don't be an arse.***
