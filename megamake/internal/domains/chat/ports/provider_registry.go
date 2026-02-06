package ports

import "strings"

// ProviderRegistry resolves providers by name (e.g., "openai", "gemini", ...).
//
// This keeps the app/service layer independent of concrete provider implementations.
type ProviderRegistry interface {
	// Get returns the provider for the given name (case-insensitive, trimmed).
	Get(name string) (Provider, bool)

	// Names returns a stable sorted list of registered provider names.
	Names() []string

	// Default returns the default provider (never nil).
	Default() Provider
}

func NormalizeProviderName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
