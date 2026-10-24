// Copyright (c) 2026 AllIsA1.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// cachedMap keeps, per subscription id, the lines a module adds. The data is read in the background and
// refreshed when it is older than ttl or when a module says it changed, so a subscription request never
// waits for the panel (except the very first one, and then only briefly).
type cachedMap struct {
	name          string
	load          func(ctx context.Context) (map[string][]string, error)
	now           func() time.Time
	ttl           time.Duration // how long a load is fresh
	retry         time.Duration // how long to wait after a failed load
	firstLoadWait time.Duration // how long the very first request waits for the first load

	mu       sync.Mutex
	loaded   bool
	stale    bool
	loadedAt time.Time
	failedAt time.Time
	data     map[string][]string
	running  chan struct{} // closed when the running load ends; nil when none runs
}

func newCachedMap(name string, load func(ctx context.Context) (map[string][]string, error)) *cachedMap {
	return &cachedMap{name: name, load: load, now: time.Now, ttl: 30 * time.Second, retry: 15 * time.Second, firstLoadWait: 200 * time.Millisecond}
}

// invalidate says the data changed: the next request starts a refresh (and is still answered from memory).
func (c *cachedMap) invalidate() {
	c.mu.Lock()
	c.stale = true
	c.failedAt = time.Time{}
	c.mu.Unlock()
}

// linksFor returns the lines of a subscription, starting a load when the data is missing or old.
func (c *cachedMap) linksFor(subID string) []string {
	c.mu.Lock()
	if !c.loaded {
		wait := c.startLoadLocked()
		c.mu.Unlock()
		if wait != nil {
			select {
			case <-wait:
			case <-time.After(c.firstLoadWait):
			}
		}
		c.mu.Lock()
	} else if c.stale || c.now().Sub(c.loadedAt) > c.ttl {
		c.startLoadLocked()
	}
	defer c.mu.Unlock()
	return c.data[subID]
}

// startLoadLocked starts a background load unless one runs or the last one failed a moment ago. It returns
// the channel that closes when the running load ends, nil when there is nothing to wait for.
func (c *cachedMap) startLoadLocked() <-chan struct{} {
	if c.running != nil {
		return c.running
	}
	if !c.failedAt.IsZero() && c.now().Sub(c.failedAt) < c.retry {
		return nil
	}
	done := make(chan struct{})
	c.running = done
	c.stale = false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
		defer cancel()
		data, err := c.load(ctx)
		c.mu.Lock()
		if err != nil {
			log.Printf("better-lucx: %s: reading the panel: %v", c.name, err)
			c.failedAt = c.now()
			c.stale = true
		} else {
			c.data, c.loaded, c.loadedAt, c.failedAt = data, true, c.now(), time.Time{}
		}
		c.running = nil
		c.mu.Unlock()
		close(done)
	}()
	return done
}
