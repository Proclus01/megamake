package adapters

import (
	"strings"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// HeuristicTokenCounter is a best-effort token estimator.
//
// v1 strategy (intentionally simple and fast):
// - approximate tokens ≈ len(text)/4 (common rough heuristic for English-like text)
// - clamp minimum to 0
// - mark Approx=true
//
// This does not attempt to be model-accurate; it's a cross-provider fallback and sanity check.
type HeuristicTokenCounter struct{}

func NewHeuristicTokenCounter() HeuristicTokenCounter { return HeuristicTokenCounter{} }

func (HeuristicTokenCounter) Count(req ports.TokenCountRequest) contractchat.TokenUsageV1 {
	in := approxTokens(req.InputText)
	out := approxTokens(req.OutputText)
	total := in + out

	return contractchat.TokenUsageV1{
		InputTokens:  intPtr(in),
		OutputTokens: intPtr(out),
		TotalTokens:  intPtr(total),
		Approx:       true,
		Notes:        noteFor(req.Provider, req.Model),
	}
}

func approxTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Use bytes length; this is intentionally crude but stable.
	n := len(s)
	t := n / 4
	if t < 1 {
		t = 1
	}
	return t
}

func noteFor(provider, model string) string {
	p := strings.TrimSpace(provider)
	m := strings.TrimSpace(model)
	if p == "" && m == "" {
		return "heuristic: tokens ~= len(text)/4"
	}
	if p == "" {
		return "heuristic: tokens ~= len(text)/4 (model=" + m + ")"
	}
	if m == "" {
		return "heuristic: tokens ~= len(text)/4 (provider=" + p + ")"
	}
	return "heuristic: tokens ~= len(text)/4 (provider=" + p + ", model=" + m + ")"
}

func intPtr(n int) *int { return &n }
