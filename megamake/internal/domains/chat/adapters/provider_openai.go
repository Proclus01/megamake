package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	contractchat "github.com/megamake/megamake/internal/contracts/v1/chat"
	"github.com/megamake/megamake/internal/domains/chat/ports"
)

// OpenAIProvider implements ports.Provider using the OpenAI HTTP API.
//
// v1 notes:
// - Verify() is implemented as "can we list models" (lightweight read-only call).
// - Streaming is handled via the Responses API with stream=true and SSE parsing.
// - Token usage is taken from provider usage when available.
type OpenAIProvider struct {
	// BaseURL defaults to "https://api.openai.com".
	// You may override via env var OPENAI_BASE_URL (useful for proxies).
	BaseURL string

	// HTTPClient is used for non-streaming calls. If nil, a default client is used.
	HTTPClient *http.Client

	// StreamClient is used for streaming calls. If nil, a default client is used.
	StreamClient *http.Client

	// APIKeyEnvVar defaults to "OPENAI_API_KEY".
	APIKeyEnvVar string
}

func NewOpenAIProvider() OpenAIProvider {
	return OpenAIProvider{
		BaseURL:      "",
		HTTPClient:   nil,
		StreamClient: nil,
		APIKeyEnvVar: "OPENAI_API_KEY",
	}
}

func (p OpenAIProvider) Name() string { return "openai" }

func (p OpenAIProvider) NetworkHosts() []string {
	return []string{"api.openai.com"}
}

func (p OpenAIProvider) Verify(ctx context.Context) (ports.VerifyResult, error) {
	_, err := p.ListModels(ctx)
	if err != nil {
		return ports.VerifyResult{OK: false, Message: err.Error()}, err
	}
	return ports.VerifyResult{OK: true, Message: "ok"}, nil
}

func (p OpenAIProvider) ListModels(ctx context.Context) ([]ports.ModelInfo, error) {
	key, err := p.apiKey()
	if err != nil {
		return nil, err
	}

	u := p.baseURL() + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: list models request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := readAllLimit(resp.Body, 2_000_000)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai: list models failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var parsed struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("openai: list models parse failed: %v", err)
	}

	out := make([]ports.ModelInfo, 0, len(parsed.Data))
	for _, it := range parsed.Data {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		out = append(out, ports.ModelInfo{
			ID:          id,
			DisplayName: "",
			OwnedBy:     it.OwnedBy,
		})
	}
	return out, nil
}

func (p OpenAIProvider) Chat(ctx context.Context, req ports.ChatRequest) (ports.ChatResponse, error) {
	key, err := p.apiKey()
	if err != nil {
		return ports.ChatResponse{}, err
	}

	u := p.baseURL() + "/v1/responses"
	payload := p.buildResponsesPayload(req, false)

	b, err := json.Marshal(payload)
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(b))
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: build request: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.httpClient().Do(httpReq)
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: responses request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := readAllLimit(resp.Body, 8_000_000)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.ChatResponse{}, fmt.Errorf("openai: responses failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	text, usage := parseResponsesOutputTextAndUsage(bodyBytes)
	respID := parseResponseID(bodyBytes)

	return ports.ChatResponse{
		Text:              text,
		UsageProvider:     usage,
		ProviderRequestID: respID,
		Model:             req.Model,
	}, nil
}

func (p OpenAIProvider) StreamChat(ctx context.Context, req ports.ChatRequest, handler ports.StreamHandler) (ports.ChatResponse, error) {
	key, err := p.apiKey()
	if err != nil {
		return ports.ChatResponse{}, err
	}

	u := p.baseURL() + "/v1/responses"
	payload := p.buildResponsesPayload(req, true)

	b, err := json.Marshal(payload)
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: marshal request: %v", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(b))
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: build request: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.streamClient().Do(httpReq)
	if err != nil {
		return ports.ChatResponse{}, fmt.Errorf("openai: stream request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := readAllLimit(resp.Body, 2_000_000)
		return ports.ChatResponse{}, fmt.Errorf("openai: stream failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	if handler != nil {
		handler.OnStart()
	}

	var full strings.Builder
	var usage *contractchat.TokenUsageV1
	respID := ""

	// SSE parsing: read lines, collect "data:" blocks until blank line.
	br := bufio.NewReader(resp.Body)
	var dataBuf strings.Builder

	flush := func() error {
		raw := strings.TrimSpace(dataBuf.String())
		dataBuf.Reset()
		if raw == "" {
			return nil
		}
		if raw == "[DONE]" {
			return doneErr{}
		}

		var ev map[string]any
		if err := json.Unmarshal([]byte(raw), &ev); err != nil {
			if handler != nil {
				handler.OnError(fmt.Errorf("openai: invalid stream json: %v", err))
			}
			return nil
		}

		typ, _ := ev["type"].(string)
		switch typ {
		case "response.output_text.delta":
			if d, ok := ev["delta"].(string); ok && d != "" {
				full.WriteString(d)
				if handler != nil {
					handler.OnDelta(d)
				}
			}

		case "response.completed", "response.incomplete", "response.failed":
			if respObj, ok := ev["response"].(map[string]any); ok {
				if id, ok := respObj["id"].(string); ok && id != "" {
					respID = id
				}
				if u := parseUsageFromResponseObject(respObj); u != nil {
					usage = u
					if handler != nil {
						handler.OnUsage(*u)
					}
				}
			}
			if typ == "response.failed" {
				msg := "openai: response.failed"
				if respObj, ok := ev["response"].(map[string]any); ok {
					if eObj, ok := respObj["error"].(map[string]any); ok {
						if m, ok := eObj["message"].(string); ok && strings.TrimSpace(m) != "" {
							msg = "openai: " + m
						}
					}
				}
				if handler != nil {
					handler.OnError(fmt.Errorf(msg))
				}
				return fmt.Errorf(msg)
			}

		case "error":
			msg := "openai: stream error"
			if eObj, ok := ev["error"].(map[string]any); ok {
				if m, ok := eObj["message"].(string); ok && strings.TrimSpace(m) != "" {
					msg = "openai: " + m
				}
			}
			if handler != nil {
				handler.OnError(fmt.Errorf(msg))
			}
			return fmt.Errorf(msg)
		}

		return nil
	}

	for {
		line, err := br.ReadString('\n')
		if line != "" {
			trim := strings.TrimRight(line, "\r\n")
			if trim == "" {
				// End of event
				if err2 := flush(); err2 != nil {
					if _, ok := err2.(doneErr); ok {
						break
					}
					return ports.ChatResponse{}, err2
				}
			} else if strings.HasPrefix(trim, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trim, "data:"))
				if dataBuf.Len() > 0 {
					dataBuf.WriteString("\n")
				}
				dataBuf.WriteString(data)
			}
		}

		if err != nil {
			// EOF: flush remaining buffered event.
			_ = flush()
			break
		}
	}

	if handler != nil {
		handler.OnDone()
	}

	return ports.ChatResponse{
		Text:              full.String(),
		UsageProvider:     usage,
		ProviderRequestID: respID,
		Model:             req.Model,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Payload / parsing helpers
////////////////////////////////////////////////////////////////////////////////

func (p OpenAIProvider) buildResponsesPayload(req ports.ChatRequest, stream bool) map[string]any {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gpt-5"
	}

	var input []map[string]any
	addMsg := func(role, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": text,
		})
	}

	if req.SystemText != "" {
		addMsg("system", req.SystemText)
	}
	if req.DeveloperText != "" {
		addMsg("developer", req.DeveloperText)
	}
	for _, m := range req.Messages {
		r := strings.TrimSpace(strings.ToLower(m.Role))
		if r == "" {
			r = "user"
		}
		addMsg(r, m.Text)
	}

	payload := map[string]any{
		"model":  model,
		"input":  input,
		"stream": stream,
	}

	if req.MaxOutputTokens > 0 {
		payload["max_output_tokens"] = req.MaxOutputTokens
	}

	reasoning := map[string]any{}
	if strings.TrimSpace(string(req.Effort)) != "" {
		reasoning["effort"] = string(req.Effort)
	}
	if req.SummaryAuto {
		reasoning["summary"] = "auto"
	}
	if len(reasoning) > 0 {
		payload["reasoning"] = reasoning
	}

	switch req.TextFormat {
	case contractchat.TextFormatJSON:
		payload["text"] = map[string]any{
			"format": map[string]any{"type": "json_object"},
		}
	default:
		payload["text"] = map[string]any{
			"format": map[string]any{"type": "text"},
		}
	}

	return payload
}

func (p OpenAIProvider) baseURL() string {
	if strings.TrimSpace(p.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.openai.com"
}

func (p OpenAIProvider) apiKey() (string, error) {
	env := strings.TrimSpace(p.APIKeyEnvVar)
	if env == "" {
		env = "OPENAI_API_KEY"
	}
	k := strings.TrimSpace(os.Getenv(env))
	if k == "" {
		return "", fmt.Errorf("openai: missing API key in env var %s", env)
	}
	return k, nil
}

func (p OpenAIProvider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (p OpenAIProvider) streamClient() *http.Client {
	if p.StreamClient != nil {
		return p.StreamClient
	}
	// For streaming, avoid a hard client timeout; rely on request context cancellation.
	return &http.Client{}
}

func parseResponsesOutputTextAndUsage(body []byte) (text string, usage *contractchat.TokenUsageV1) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", nil
	}

	usage = parseUsageFromResponseObject(root)

	outAny, _ := root["output"].([]any)
	var sb strings.Builder
	for _, item := range outAny {
		it, _ := item.(map[string]any)
		if it == nil {
			continue
		}
		if t, _ := it["type"].(string); t != "message" {
			continue
		}
		contentAny, _ := it["content"].([]any)
		for _, part := range contentAny {
			pm, _ := part.(map[string]any)
			if pm == nil {
				continue
			}
			if pt, _ := pm["type"].(string); pt == "output_text" {
				if tx, _ := pm["text"].(string); tx != "" {
					sb.WriteString(tx)
				}
			}
		}
	}

	return sb.String(), usage
}

func parseUsageFromResponseObject(respObj map[string]any) *contractchat.TokenUsageV1 {
	uAny, ok := respObj["usage"].(map[string]any)
	if !ok || uAny == nil {
		return nil
	}
	inTok := intFromAny(uAny["input_tokens"])
	outTok := intFromAny(uAny["output_tokens"])
	totTok := intFromAny(uAny["total_tokens"])
	if inTok == nil && outTok == nil && totTok == nil {
		return nil
	}
	return &contractchat.TokenUsageV1{
		InputTokens:  inTok,
		OutputTokens: outTok,
		TotalTokens:  totTok,
		Approx:       false,
		Notes:        "provider: openai usage",
	}
}

func parseResponseID(body []byte) string {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return ""
	}
	if id, _ := root["id"].(string); id != "" {
		return id
	}
	return ""
}

func intFromAny(v any) *int {
	switch x := v.(type) {
	case float64:
		n := int(x)
		return &n
	case int:
		n := x
		return &n
	case json.Number:
		if i64, err := x.Int64(); err == nil {
			n := int(i64)
			return &n
		}
	}
	return nil
}

func readAllLimit(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = 1_000_000
	}
	return io.ReadAll(io.LimitReader(r, max))
}

type doneErr struct{}

func (doneErr) Error() string { return "done" }
