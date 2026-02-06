package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/megamake/megamake/internal/domains/chat/ports"
	"github.com/megamake/megamake/internal/platform/policy"
)

// VerifyProviderRequest verifies API connectivity/credentials for a provider.
type VerifyProviderRequest struct {
	Provider string

	TimeoutSeconds int

	// Network policy (enforced before any provider network calls).
	NetEnabled   bool
	AllowDomains []string
}

type VerifyProviderResult struct {
	Provider string             `json:"provider"`
	OK       bool               `json:"ok"`
	Message  string             `json:"message,omitempty"`
	Identity ports.VerifyResult `json:"identity"`
}

// ListModelsRequest lists models for a provider.
type ListModelsRequest struct {
	Provider string
	Limit    int

	TimeoutSeconds int

	// Network policy (enforced before any provider network calls).
	NetEnabled   bool
	AllowDomains []string

	// Cache behavior (server-side, in-memory):
	// - CacheTTLSeconds <=0 uses default TTL (300s).
	// - NoCache forces a fresh provider call and overwrites the cache on success.
	CacheTTLSeconds int  `json:"cache_ttl_seconds,omitempty"`
	NoCache         bool `json:"no_cache,omitempty"`
}

type ListModelsResult struct {
	Provider string            `json:"provider"`
	Models   []ports.ModelInfo `json:"models"`

	// Cache info for UI/debugging
	Cached    bool   `json:"cached"`
	CachedAt  string `json:"cached_at,omitempty"` // RFC3339Nano UTC
	CacheAgeS int    `json:"cache_age_s,omitempty"`
}

// VerifyProvider checks if a provider is usable (e.g. API key valid).
func (s *Service) VerifyProvider(req VerifyProviderRequest) (VerifyProviderResult, error) {
	if s.Providers == nil {
		return VerifyProviderResult{}, fmt.Errorf("internal error: chat Providers registry is nil")
	}

	name := strings.TrimSpace(req.Provider)
	p, ok := s.Providers.Get(name)
	if !ok || p == nil {
		return VerifyProviderResult{}, fmt.Errorf("unknown provider: %s", name)
	}

	// Enforce network policy for network providers.
	if err := requireProviderAllowed(req.NetEnabled, req.AllowDomains, p); err != nil {
		res := VerifyProviderResult{
			Provider: p.Name(),
			OK:       false,
			Message:  err.Error(),
			Identity: ports.VerifyResult{OK: false, Message: err.Error()},
		}
		return res, err
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vr, err := p.Verify(ctx)
	if err != nil {
		return VerifyProviderResult{
			Provider: p.Name(),
			OK:       false,
			Message:  err.Error(),
			Identity: vr,
		}, err
	}

	return VerifyProviderResult{
		Provider: p.Name(),
		OK:       vr.OK,
		Message:  vr.Message,
		Identity: vr,
	}, nil
}

// ListModels returns a provider's available models (best-effort).
// Server-side caching:
// - If ModelCache is available and NoCache=false, return cached models if fresh.
// - Otherwise call provider and refresh cache on success.
func (s *Service) ListModels(req ListModelsRequest) (ListModelsResult, error) {
	if s.Providers == nil {
		return ListModelsResult{}, fmt.Errorf("internal error: chat Providers registry is nil")
	}

	name := strings.TrimSpace(req.Provider)
	p, ok := s.Providers.Get(name)
	if !ok || p == nil {
		return ListModelsResult{}, fmt.Errorf("unknown provider: %s", name)
	}

	// Enforce network policy for network providers.
	if err := requireProviderAllowed(req.NetEnabled, req.AllowDomains, p); err != nil {
		return ListModelsResult{}, err
	}

	ttlS := req.CacheTTLSeconds
	if ttlS <= 0 {
		ttlS = 300
	}
	if ttlS < 5 {
		ttlS = 5
	}
	if ttlS > 3600 {
		ttlS = 3600
	}
	ttl := time.Duration(ttlS) * time.Second

	// Cache lookup (if enabled and cache exists)
	if !req.NoCache && s.ModelCache != nil {
		models, cachedAt, ok := s.ModelCache.Get(p.Name())
		if ok && !cachedAt.IsZero() {
			age := time.Since(cachedAt)
			if age < ttl {
				models = applyModelLimit(models, req.Limit)
				return ListModelsResult{
					Provider:  p.Name(),
					Models:    models,
					Cached:    true,
					CachedAt:  cachedAt.UTC().Format(time.RFC3339Nano),
					CacheAgeS: int(age.Seconds()),
				}, nil
			}
		}
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	models, err := p.ListModels(ctx)
	if err != nil {
		return ListModelsResult{}, err
	}

	// Stable sort by ID
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	// Update cache on success
	if s.ModelCache != nil {
		_ = s.ModelCache.Put(p.Name(), models, time.Now().UTC())
	}

	models = applyModelLimit(models, req.Limit)
	return ListModelsResult{
		Provider: p.Name(),
		Models:   models,
		Cached:   false,
	}, nil
}

func applyModelLimit(models []ports.ModelInfo, limit int) []ports.ModelInfo {
	limitN := limit
	if limitN <= 0 {
		limitN = 200
	}
	if limitN > 5000 {
		limitN = 5000
	}
	if len(models) > limitN {
		return models[:limitN]
	}
	return models
}

func requireProviderAllowed(netEnabled bool, allowDomains []string, p ports.Provider) error {
	if p == nil {
		return fmt.Errorf("nil provider")
	}
	hosts := p.NetworkHosts()
	if len(hosts) == 0 {
		// Local/offline provider: no network policy needed.
		return nil
	}

	pol := policy.Policy{
		NetEnabled:   netEnabled,
		AllowDomains: allowDomains,
	}

	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if err := pol.RequireNetworkAllowed(h); err != nil {
			return err
		}
	}
	return nil
}
