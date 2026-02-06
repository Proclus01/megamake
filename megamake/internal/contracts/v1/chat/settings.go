package chat

// TextFormatV1 is a hint for how the assistant should format its output.
// Not all providers support all formats; adapters may ignore unsupported values.
type TextFormatV1 string

const (
	TextFormatText     TextFormatV1 = "text"
	TextFormatMarkdown TextFormatV1 = "markdown"
	TextFormatJSON     TextFormatV1 = "json"
)

// VerbosityV1 is a hint for response verbosity.
type VerbosityV1 string

const (
	VerbosityLow    VerbosityV1 = "low"
	VerbosityMedium VerbosityV1 = "medium"
	VerbosityHigh   VerbosityV1 = "high"
)

// EffortV1 is a hint for reasoning effort.
// Providers may ignore this; it is still valuable to record consistently.
type EffortV1 string

const (
	EffortMinimal EffortV1 = "minimal"
	EffortLow     EffortV1 = "low"
	EffortMedium  EffortV1 = "medium"
	EffortHigh    EffortV1 = "high"
)

// ToolsV1 indicates which tool categories are enabled for a request.
// These are high-level toggles; the provider adapter decides how (or whether) to map them.
type ToolsV1 struct {
	WebSearch       bool `json:"web_search,omitempty"`
	CodeInterpreter bool `json:"code_interpreter,omitempty"`
	FileSearch      bool `json:"file_search,omitempty"`
	ImageGeneration bool `json:"image_generation,omitempty"`
}

// SettingsV1 represents persisted, non-secret chat settings.
//
// This is intended to be stored in:
//
//	<artifactDir>/MEGACHAT/settings.json
//
// Secrets (API keys) must NOT be stored here; they should come from environment variables.
type SettingsV1 struct {
	// Provider is a stable name like: "openai", "gemini", "anthropic", "grok", "local".
	Provider string `json:"provider,omitempty"`

	// Model is the provider model identifier, e.g. "gpt-5.2" (provider-specific).
	Model string `json:"model,omitempty"`

	// Defaults for new runs (can be overridden per run).
	SystemText    string `json:"systemText,omitempty"`
	DeveloperText string `json:"developerText,omitempty"`

	// Output preferences (best-effort)
	TextFormat  TextFormatV1 `json:"textFormat,omitempty"`
	Verbosity   VerbosityV1  `json:"verbosity,omitempty"`
	Effort      EffortV1     `json:"effort,omitempty"`
	SummaryAuto bool         `json:"summaryAuto,omitempty"`

	// MaxOutputTokens is a per-response limit (provider-specific; adapters may clamp/ignore).
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`

	Tools ToolsV1 `json:"tools,omitempty"`

	// UpdatedTS is when these settings were last updated (RFC3339Nano).
	UpdatedTS string `json:"updated_ts,omitempty"`
}
