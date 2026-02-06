package adapters

import (
	"context"
	"sort"
	"strings"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// ProviderRegistry is the default in-process provider registry.
// It registers concrete provider adapters (OpenAI today; others later)
// and provides a safe default provider.
type ProviderRegistry struct {
	items   map[string]ports.Provider
	defName string
}

func NewProviderRegistry() ProviderRegistry {
	openai := NewOpenAIProvider()
	stub := NewStubProvider()

	items := map[string]ports.Provider{
		openai.Name(): openai,
		stub.Name():   stub,
	}

	// Default to stub until the app/service chooses based on settings.
	return ProviderRegistry{
		items:   items,
		defName: stub.Name(),
	}
}

func (r ProviderRegistry) Get(name string) (ports.Provider, bool) {
	if r.items == nil {
		return NewStubProvider(), true
	}
	key := ports.NormalizeProviderName(name)
	if key == "" {
		p := r.Default()
		return p, true
	}
	p, ok := r.items[key]
	return p, ok
}

func (r ProviderRegistry) Names() []string {
	if len(r.items) == 0 {
		return []string{NewStubProvider().Name()}
	}
	out := make([]string, 0, len(r.items))
	for k := range r.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r ProviderRegistry) Default() ports.Provider {
	if r.items == nil || len(r.items) == 0 {
		return NewStubProvider()
	}
	name := ports.NormalizeProviderName(r.defName)
	if name != "" {
		if p, ok := r.items[name]; ok && p != nil {
			return p
		}
	}
	// stable fallback: return lexicographically-first provider
	names := r.Names()
	if len(names) > 0 {
		if p, ok := r.items[names[0]]; ok && p != nil {
			return p
		}
	}
	return NewStubProvider()
}

////////////////////////////////////////////////////////////////////////////////
// Stub provider (safe default, no network)
////////////////////////////////////////////////////////////////////////////////

// StubProvider is a local, deterministic provider used for early UI/flow validation
// and as a safe default when no real provider is configured.
type StubProvider struct{}

func NewStubProvider() StubProvider { return StubProvider{} }

func (StubProvider) Name() string { return "stub" }

func (StubProvider) NetworkHosts() []string { return nil }

func (StubProvider) Verify(ctx context.Context) (ports.VerifyResult, error) {
	_ = ctx
	return ports.VerifyResult{OK: true, Message: "stub provider: ok"}, nil
}

func (StubProvider) ListModels(ctx context.Context) ([]ports.ModelInfo, error) {
	_ = ctx
	return []ports.ModelInfo{
		{ID: "stub-model", DisplayName: "Stub Model", OwnedBy: "megamake"},
	}, nil
}

func (StubProvider) Chat(ctx context.Context, req ports.ChatRequest) (ports.ChatResponse, error) {
	_ = ctx
	user := lastUserText(req.Messages)
	txt := "Stub assistant reply (provider not wired yet).\n\nYou said:\n" + user + "\n"
	return ports.ChatResponse{
		Text:              txt,
		UsageProvider:     nil,
		ProviderRequestID: "",
		Model:             req.Model,
	}, nil
}

func (StubProvider) StreamChat(ctx context.Context, req ports.ChatRequest, handler ports.StreamHandler) (ports.ChatResponse, error) {
	_ = ctx

	res, _ := StubProvider{}.Chat(ctx, req)

	if handler != nil {
		handler.OnStart()
	}

	// Stream the response in small chunks.
	chunks := chunkStub(res.Text, 24)
	for _, c := range chunks {
		if handler != nil && c != "" {
			handler.OnDelta(c)
		}
	}

	// Provide a small fake usage (marked approx=false because it's "provider" side,
	// but the values are not real; better to omit entirely).
	_ = contractchat.TokenUsageV1{} // keep contract import useful; real providers will populate this.

	if handler != nil {
		handler.OnDone()
	}

	return res, nil
}

func lastUserText(msgs []ports.ChatMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	// Find last "user" role message.
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.ToLower(strings.TrimSpace(msgs[i].Role)) == "user" {
			return strings.TrimSpace(msgs[i].Text)
		}
	}
	// fallback: last message text
	return strings.TrimSpace(msgs[len(msgs)-1].Text)
}

func chunkStub(s string, n int) []string {
	if n <= 0 || s == "" {
		return []string{s}
	}
	var out []string
	for len(s) > 0 {
		if len(s) <= n {
			out = append(out, s)
			break
		}
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}
