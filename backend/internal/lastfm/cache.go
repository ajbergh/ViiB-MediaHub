// Package lastfm provides Last.FM API integration for ViiB MediaHub.
//
// cache.go - Thread-safe LRU cache for Last.FM API responses.
//
// This file implements a simple time-based LRU cache to reduce API calls
// and improve response times. Features:
//   - Configurable maximum size (default 10,000 entries)
//   - Configurable TTL (default 24 hours)
//   - Thread-safe operations via sync.RWMutex
//   - Automatic expiration of stale entries
//   - LRU eviction when at capacity
//
// Cache keys follow the pattern: "type:artist:track" or "type:artist:limit"
// where type is one of: track, tracktags, similar, artist, similarartist, album
//
// Created: 2025-12-31
// Last Modified: 2025-12-31
package lastfm

import (
	"strings"
	"sync"
	"time"
)

// Cache provides thread-safe LRU caching for Last.FM API responses.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	value     interface{}
	createdAt time.Time
}

// NewCache creates a new cache with the given max size and TTL.
func NewCache(maxSize int, ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a value from the cache.
// Returns the value and true if found and not expired, otherwise nil and false.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Since(entry.createdAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in the cache.
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		value:     value,
		createdAt: time.Now(),
	}
}

// evictOldest removes the oldest entry from the cache.
// Must be called with lock held.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*cacheEntry)
	c.mu.Unlock()
}

// Size returns the number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// normalizeKey normalizes a string for use as a cache key.
func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
