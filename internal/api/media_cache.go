package api

import "sync"

// cacheState is the part of the file details cache that is read without
// waiting for a build: the Library lists ask for the cached lists on every
// load, and while the lists are being rebuilt (which on a big library takes
// seconds, mostly reading every folder on the disks) a request gets the
// previous ones instead of queuing behind the build. mediaInfoState.mu only
// serialises builds.
type cacheState struct {
	mu                 sync.Mutex
	sum                *mediaSummaries
	inc                *incompleteSet
	renewingSummaries  bool
	renewingIncomplete bool
}

func (c *cacheState) summaries() *mediaSummaries {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sum
}

func (c *cacheState) setSummaries(v *mediaSummaries) {
	c.mu.Lock()
	c.sum = v
	c.mu.Unlock()
}

func (c *cacheState) incomplete() *incompleteSet {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inc
}

func (c *cacheState) setIncomplete(v *incompleteSet) {
	c.mu.Lock()
	c.inc = v
	c.mu.Unlock()
}

// renew runs build in the background unless a renewal marked by flag is
// already running.
func (c *cacheState) renew(flag *bool, build func()) {
	c.mu.Lock()
	if *flag {
		c.mu.Unlock()
		return
	}
	*flag = true
	c.mu.Unlock()
	go func() {
		defer func() {
			c.mu.Lock()
			*flag = false
			c.mu.Unlock()
		}()
		build()
	}()
}
