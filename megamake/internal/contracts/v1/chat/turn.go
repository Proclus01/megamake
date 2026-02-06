package chat

// TurnMetricsV1 is written to turn_###.json for each turn.
// It captures structured per-turn metrics and is intended for debugging,
// auditing, and UI display (durations, tokens, errors).
type TurnMetricsV1 struct {
	// Turn is 1-based.
	Turn int `json:"turn"`

	RunName string `json:"run_name,omitempty"`

	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`

	// Settings is the effective, non-secret settings snapshot used for this turn.
	// This is intentionally duplicated per turn for auditability/reproducibility.
	//
	// Notes:
	// - Providers may ignore some settings; we still record what we *requested*.
	// - Secrets (API keys) are NOT included in SettingsV1.
	Settings *SettingsV1 `json:"settings,omitempty"`

	// Timestamps are RFC3339Nano (UTC recommended).
	StartedTS   string `json:"started_ts,omitempty"`
	FirstByteTS string `json:"first_byte_ts,omitempty"`
	CompletedTS string `json:"completed_ts,omitempty"`

	// Durations in milliseconds (best-effort).
	TTFBMs  *int `json:"ttfb_ms,omitempty"`
	TotalMs *int `json:"total_ms,omitempty"`

	// Token usage (best-effort).
	UsageProvider *TokenUsageV1 `json:"usage_provider,omitempty"`
	UsageInternal *TokenUsageV1 `json:"usage_internal,omitempty"`

	// ProviderRequestID may be populated when a provider returns a request-id,
	// trace id, or other correlation identifier.
	ProviderRequestID string `json:"provider_request_id,omitempty"`

	// Error is a short human-readable string if the turn failed.
	Error string `json:"error,omitempty"`
}
