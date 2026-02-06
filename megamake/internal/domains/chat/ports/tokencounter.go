package ports

import (
	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
)

// TokenCounter provides internal token counting/estimation.
//
// Your requirement is:
// - store provider-reported usage when available, AND
// - store our own internal usage numbers as well.
//
// This port enables multiple strategies:
// - heuristic estimator (v1)
// - exact model tokenizers (future)
type TokenCounter interface {
	Count(req TokenCountRequest) contractchat.TokenUsageV1
}

type TokenCountRequest struct {
	Provider string
	Model    string

	// InputText is the effective prompt content (system+developer+prior messages+user message).
	// OutputText is the assistant response text.
	//
	// For streaming, OutputText may be partial; you can count at end for final metrics.
	InputText  string
	OutputText string
}
