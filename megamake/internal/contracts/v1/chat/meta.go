package chat

// TokenUsageV1 captures token accounting for a single request/turn.
// Providers may omit some or all fields; internal counters may provide estimates.
// If Approx is true, the values are estimates (not authoritative).
type TokenUsageV1 struct {
	InputTokens  *int   `json:"inputTokens,omitempty"`
	OutputTokens *int   `json:"outputTokens,omitempty"`
	TotalTokens  *int   `json:"totalTokens,omitempty"`
	Approx       bool   `json:"approx,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// MetaV1 is the on-disk meta.json contract for a chat run.
// It is updated over time as messages/turns are appended.
//
// Timestamps should be RFC3339Nano (UTC recommended).
type MetaV1 struct {
	RunName string `json:"run_name"`

	Title         string `json:"title"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	SystemText    string `json:"systemText,omitempty"`
	DeveloperText string `json:"developerText,omitempty"`

	CreatedTS string `json:"created_ts"`
	UpdatedTS string `json:"updated_ts"`

	MessagesN int `json:"messages_n"`
	TurnsN    int `json:"turns_n"`

	// Last turn usage (best-effort)
	LastUsageProvider *TokenUsageV1 `json:"last_usage_provider,omitempty"`
	LastUsageInternal *TokenUsageV1 `json:"last_usage_internal,omitempty"`

	// Totals across the conversation (best-effort)
	TotalUsageProvider *TokenUsageV1 `json:"total_usage_provider,omitempty"`
	TotalUsageInternal *TokenUsageV1 `json:"total_usage_internal,omitempty"`

	// Latency metrics for last assistant response (ms).
	LastTTFBMs  *int `json:"last_ttfb_ms,omitempty"`
	LastTotalMs *int `json:"last_total_ms,omitempty"`

	// A short error string for the last failure (if any). Keep it human-readable.
	LastError string `json:"last_error,omitempty"`
}
