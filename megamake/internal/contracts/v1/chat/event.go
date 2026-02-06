package chat

// RoleV1 is the message author role for transcript events.
type RoleV1 string

const (
	RoleUser      RoleV1 = "user"
	RoleAssistant RoleV1 = "assistant"
	RoleSystem    RoleV1 = "system"
	RoleDeveloper RoleV1 = "developer"
)

// TranscriptEventV1 is a single append-only event written to transcript.jsonl.
//
// This contract is intentionally small and stable. It matches the spirit of your
// prior jsonl samples:
//
//	{"role":"user","text":"...","ts":"...Z"}
//	{"role":"assistant","text":"...","ts":"...Z"}
//
// Additive fields (turn, provider, usage, etc.) are optional and can be ignored
// by older tooling.
type TranscriptEventV1 struct {
	Role RoleV1 `json:"role"`
	Text string `json:"text"`
	TS   string `json:"ts"` // RFC3339Nano (UTC recommended)

	// Turn is 1-based. For system/developer events you may set Turn=0.
	Turn int `json:"turn,omitempty"`

	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`

	// Usage (best-effort). Provider usage is authoritative when present.
	UsageProvider *TokenUsageV1 `json:"usage_provider,omitempty"`
	UsageInternal *TokenUsageV1 `json:"usage_internal,omitempty"`
}
