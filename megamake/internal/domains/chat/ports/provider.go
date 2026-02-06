package ports

import (
	"context"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
)

// Provider is the outbound port for LLM providers (OpenAI, Gemini, Claude, Grok, local, etc.).
//
// Chat app logic (runs, turns, persistence, jobs, metrics) must not depend on a specific
// provider implementation; it talks only to this interface.
//
// Network policy enforcement is done by the app/service layer, using NetworkHosts().
type Provider interface {
	// Name is a stable identifier like "openai", "gemini", "anthropic", "grok", "local".
	Name() string

	// NetworkHosts returns the hostnames this provider will contact for API calls.
	// - For network providers, this should include e.g. "api.openai.com".
	// - For local/offline providers, return nil or an empty slice.
	NetworkHosts() []string

	// Verify checks whether the provider is usable (e.g. API key valid).
	Verify(ctx context.Context) (VerifyResult, error)

	// ListModels returns the provider's available models (best-effort).
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// Chat performs a non-streaming chat completion (best-effort).
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)

	// StreamChat performs a streaming chat completion (best-effort).
	// Implementations should call handler callbacks as output arrives.
	StreamChat(ctx context.Context, req ChatRequest, handler StreamHandler) (ChatResponse, error)
}

// VerifyResult is returned by Provider.Verify.
type VerifyResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`

	// Provider-specific identity hints (optional).
	Account string `json:"account,omitempty"`
	Org     string `json:"org,omitempty"`
}

// ModelInfo describes an available model.
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	OwnedBy     string `json:"ownedBy,omitempty"`
}

// ChatMessage is a single message in a chat request.
type ChatMessage struct {
	Role string `json:"role"` // "system" | "developer" | "user" | "assistant"
	Text string `json:"text"`
}

// ChatRequest is the provider-facing request for a single assistant response.
// The app/service is responsible for converting a chat run state into this request.
type ChatRequest struct {
	Model string `json:"model,omitempty"`

	// These are convenience fields; adapters may map them to provider-specific message formats.
	SystemText    string `json:"systemText,omitempty"`
	DeveloperText string `json:"developerText,omitempty"`

	// Messages should include prior turns if you want conversational context.
	// Typical shape:
	//   - user/assistant alternating
	Messages []ChatMessage `json:"messages,omitempty"`

	// Output preferences (best-effort; adapters may ignore unsupported fields).
	TextFormat      contractchat.TextFormatV1 `json:"textFormat,omitempty"`
	Verbosity       contractchat.VerbosityV1  `json:"verbosity,omitempty"`
	Effort          contractchat.EffortV1     `json:"effort,omitempty"`
	SummaryAuto     bool                      `json:"summaryAuto,omitempty"`
	MaxOutputTokens int                       `json:"maxOutputTokens,omitempty"`

	Tools contractchat.ToolsV1 `json:"tools,omitempty"`
}

// ChatResponse is the provider-facing result for a single assistant response.
type ChatResponse struct {
	Text string `json:"text"`

	// Provider usage is authoritative when present.
	UsageProvider *contractchat.TokenUsageV1 `json:"usage_provider,omitempty"`

	ProviderRequestID string `json:"provider_request_id,omitempty"`
	Model             string `json:"model,omitempty"`
}

// StreamHandler receives streaming callbacks.
// Implementations should be fast and non-blocking; heavy work should be done elsewhere.
type StreamHandler interface {
	// OnStart is called when streaming begins (optional).
	OnStart()

	// OnDelta is called as text chunks arrive.
	OnDelta(textDelta string)

	// OnUsage may be called when usage becomes available (often only at end).
	OnUsage(usage contractchat.TokenUsageV1)

	// OnError is called if streaming fails (optional; StreamChat also returns an error).
	OnError(err error)

	// OnDone is called when streaming finishes successfully (optional).
	OnDone()
}
