package adapters

import (
	"sort"
	"sync"
	"time"

	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// MemoryModelCache is an in-memory, concurrency-safe cache for provider model lists.
// Intended for long-running server usage.
type MemoryModelCache struct {
	mu    sync.Mutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	models   []ports.ModelInfo
	cachedAt time.Time
}

func NewMemoryModelCache() *MemoryModelCache {
	return &MemoryModelCache{
		items: map[string]cacheEntry{},
	}
}

func (c *MemoryModelCache) Get(provider string) (models []ports.ModelInfo, cachedAt time.Time, ok bool) {
	if c == nil {
		return nil, time.Time{}, false
	}
	key := ports.NormalizeProviderName(provider)
	if key == "" {
		return nil, time.Time{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	ent, ok := c.items[key]
	if !ok {
		return nil, time.Time{}, false
	}

	// Return a copy so callers can’t mutate cache.
	out := append([]ports.ModelInfo(nil), ent.models...)
	return out, ent.cachedAt, true
}

func (c *MemoryModelCache) Put(provider string, models []ports.ModelInfo, cachedAt time.Time) error {
	if c == nil {
		return nil
	}
	key := ports.NormalizeProviderName(provider)
	if key == "" {
		return nil
	}
	if cachedAt.IsZero() {
		cachedAt = time.Now().UTC()
	}

	// Store a stable sorted copy
	cp := append([]ports.ModelInfo(nil), models...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].ID < cp[j].ID })

	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = cacheEntry{models: cp, cachedAt: cachedAt}
	return nil
}

func (c *MemoryModelCache) Clear(provider string) error {
	if c == nil {
		return nil
	}
	key := ports.NormalizeProviderName(provider)
	if key == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
	return nil
}

func (c *MemoryModelCache) ClearAll() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = map[string]cacheEntry{}
	return nil
}
