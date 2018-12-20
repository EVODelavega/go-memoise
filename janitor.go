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
			// we don't want to risk a race condition while we're doing maintenance, the channels might be full
			drainCtx, cfunc := context.WithCancel(ctx)
			// this ensures the sch and dch don't cause deadlocks
			// use channel to reassign map within the same routine, too
			mch := make(chan map[string]struct{}, 1)
			j.chanDrain(drainCtx, mch)
			for k := range j.managedKeys {
				// quickly lock, get value && unlock
				j.c.mu.RLock()
				e, ok := j.c.entries[k]
				j.c.mu.RUnlock()
				if !ok {
					// key doesn't exist anymore, remove from managed key set
					j.dch <- k
					continue
				}
				e.mu.Lock()
				// this func messes around with the entry, so acquire lock
				j.refreshItem(e, now, k)
				e.mu.Unlock()
			}
			cfunc() // and cancel the chanDrain routine
			j.managedKeys = <-mch
			close(mch)
		}
	}
}

func (j *janitor) chanDrain(ctx context.Context, ch chan<- map[string]struct{}) {
	mapCpy := map[string]struct{}{}
	// create a copy of the managedKeys map, avoiding race conditions
	for k, s := range j.managedKeys {
		mapCpy[k] = s
	}
	// keep consuming channels, update copy of map used in start routine
	go func() {
		for {
			select {
			case <-ctx.Done():
				// now the management has completed, we can reassign the updated map
				ch <- mapCpy
				return
			case k := <-j.dch:
				delete(mapCpy, k)
			case k := <-j.sch:
				mapCpy[k] = struct{}{}
			}
		}
	}()
}

func (j *janitor) refreshItem(e *centry, now time.Time, k string) {
	if e.item.expires.After(now) {
		return
	}
	if e.rt == NoRefresh {
		j.dch <- k
		return
	}
	v, err := e.cb()
	// refresh in cache, we have locked the item
	j.c.autoRefresh(k, v, err)
}
