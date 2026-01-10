package media

import (
	"sync"
)

// DurationCache caches media file durations to avoid repeated ffprobe calls.
type DurationCache struct {
	cache map[string]float64
	mu    sync.RWMutex
}

// NewDurationCache creates a new duration cache.
func NewDurationCache() *DurationCache {
	return &DurationCache{
		cache: make(map[string]float64),
	}
}

// Get retrieves a cached duration.
func (c *DurationCache) Get(path string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	duration, ok := c.cache[path]
	return duration, ok
}

// Set stores a duration in the cache.
func (c *DurationCache) Set(path string, duration float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[path] = duration
}

// Clear removes all cached durations.
func (c *DurationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]float64)
}

// Remove removes a specific path from the cache.
func (c *DurationCache) Remove(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, path)
}

// Size returns the number of cached entries.
func (c *DurationCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
