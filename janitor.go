package memoise

import (
	"context"
	"time"
)

type janitor struct {
	cfunc       context.CancelFunc  // this is so we can control the janitor without messing with cache
	c           *cache              // cache to work with, obviously
	cycle       time.Duration       // interval at which we want the janitor to kick in
	managedKeys map[string]struct{} // list of keys to manage, use map for easier lookups
	dch         chan string         // channel used to notify janitor to ignore a certain key
	sch         chan string         // channel used to notify janitor of another key to manage
}

func newJanitor(ctx context.Context, c *cache, cycle time.Duration) *janitor {
	// channels buffer to 1 -> so they're not blocking, but the risk of having a key be invalidated
	// and re-added & stuff like that should be reduced, hence don't increase buffers
	// without thinking this through, especially not the dch buffer
	return &janitor{
		c:     c,
		cycle: cycle,
		dch:   make(chan string, 1),
		sch:   make(chan string, 1),
	}
}

func (j *janitor) start(ctx context.Context) {
	// already started
	if j.cfunc != nil {
		return
	}
	ctx, j.cfunc = context.WithCancel(ctx)
	tick := time.NewTimer(j.cycle)
	for {
		select {
		case <-ctx.Done():
			close(j.dch)
			close(j.sch)
			tick.Stop()
			return
		case k := <-j.sch:
			j.managedKeys[k] = struct{}{}
		case k := <-j.dch:
			if _, ok := j.managedKeys[k]; ok {
				delete(j.managedKeys, k)
			}
		case now := <-tick.C:
			for k := range j.managedKeys {
				j.c.mu.Lock()
				if e, ok := j.c.entries[k]; ok {
					// item valid, still:
					if e.item.expires.After(now) {
						j.c.mu.Unlock()
						continue
					}
					if e.rt == NoRefresh {
						delete(j.c.entries, k)
						delete(j.managedKeys, k)
						j.c.mu.Unlock()
						continue
					}
					// @TODO refresh
					v, err := j.c.entries[k].cb()
					j.c.autoRefresh(k, v, err)
					j.c.mu.Unlock()
				}
			}
		}
	}
}
