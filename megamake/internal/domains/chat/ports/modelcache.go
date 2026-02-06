package ports

import (
	"time"
)

// ModelCache caches provider model lists in-memory (server-side).
//
// Rationale:
// - Listing models can be slow and rate-limited.
// - The server is long-running, so an in-memory cache is appropriate.
// - Cache sits in the app layer via a port to keep transport/provider adapters clean.
//
// This cache is intentionally keyed only by provider name for v1.
// (If you later support per-request API keys or multi-tenant auth, you’ll expand the key.)
type ModelCache interface {
	Get(provider string) (models []ModelInfo, cachedAt time.Time, ok bool)
	Put(provider string, models []ModelInfo, cachedAt time.Time) error
	Clear(provider string) error
	ClearAll() error
}
