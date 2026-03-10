package db

import (
	"sync"
	"time"
)

type cacheEntry struct {
	tasks     []Task
	expiresAt time.Time
}

type TaskCache struct {
	mu      sync.RWMutex
	cache   map[string]*cacheEntry
	ttl     time.Duration
	maxSize int
}

var (
	defaultTaskCache     *TaskCache
	defaultTaskCacheOnce sync.Once
)

func getDefaultTaskCache() *TaskCache {
	defaultTaskCacheOnce.Do(func() {
		defaultTaskCache = NewTaskCache(5*time.Second, 100)
	})
	return defaultTaskCache
}

func NewTaskCache(ttl time.Duration, maxSize int) *TaskCache {
	c := &TaskCache{
		cache:   make(map[string]*cacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
	go c.cleanupLoop()
	return c
}

func (c *TaskCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	for range ticker.C {
		c.cleanup()
	}
}

func (c *TaskCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, key)
		}
	}
}

func (c *TaskCache) Get(key string) ([]Task, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, exists := c.cache[key]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.tasks, true
}

func (c *TaskCache) Set(key string, tasks []Task) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= c.maxSize {
		now := time.Now()
		var oldestKey string
		oldestTime := now
		for k, v := range c.cache {
			if v.expiresAt.Before(oldestTime) {
				oldestTime = v.expiresAt
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(c.cache, oldestKey)
		}
	}
	c.cache[key] = &cacheEntry{
		tasks:     tasks,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *TaskCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

func (c *TaskCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cacheEntry)
}
