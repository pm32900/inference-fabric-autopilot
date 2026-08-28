package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal HTTP client for the Ollama REST API.
// It exposes only the two calls needed to build an OllamaSnapshot:
//   - ListRunning — lists models currently loaded in VRAM (/api/ps)
//   - Ping       — fires a zero-token generate call to get cumulative counters
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a Client pointing at the Ollama server at baseURL.
// e.g. "http://localhost:11434"
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ListRunning returns the models currently loaded in VRAM.
// Maps to GET /api/ps.
func (c *Client) ListRunning(ctx context.Context) ([]PSModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: building /api/ps request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: GET /api/ps: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: /api/ps returned status %d", resp.StatusCode)
	}

	var ps PSResponse
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, fmt.Errorf("ollama: decoding /api/ps response: %w", err)
	}

	return ps.Models, nil
}

// Snapshot builds an OllamaSnapshot for the given model by combining
// the PSModel data with a warm stats ping from /api/generate.
//
// The generate ping sends stream:false with an empty prompt. Ollama returns
// cumulative counters (PromptEvalCount, EvalCount, durations) in the response
// body. These are raw cumulative values; the collector computes rates.
//
// If the generate ping fails, the snapshot is returned with only the
// PSModel fields populated and zero counters — the collector handles this
// gracefully by not updating the rate tracker for that cycle.
func (c *Client) Snapshot(ctx context.Context, model PSModel) (OllamaSnapshot, error) {
	snap := OllamaSnapshot{
		ModelName: model.Name,
		SizeBytes: model.Size,
		SizeVRAM:  model.SizeVRAM,
		ExpiresAt: model.ExpiresAt,
		NumGPU:    model.Details.NumGPU,
		NumCPU:    model.Details.NumCPU,
	}

	stats, err := c.warmPing(ctx, model.Name)
	if err != nil {
		// Non-fatal — return partial snapshot with zero counters
		return snap, nil
	}

	snap.PromptEvalCount = stats.PromptEvalCount
	snap.EvalCount = stats.EvalCount
	snap.PromptEvalDurationNs = stats.PromptEvalDurationNs
	snap.EvalDurationNs = stats.EvalDurationNs
	snap.TotalDurationNs = stats.TotalDurationNs

	return snap, nil
}

// warmPing sends an empty-prompt generate request to collect Ollama's
// cumulative per-model counters without actually generating any tokens.
// Uses stream:false so the full response arrives in one JSON object.
func (c *Client) warmPing(ctx context.Context, model string) (GenerateResponse, error) {
	payload := fmt.Sprintf(`{"model":%q,"prompt":"","stream":false}`, model)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		c.baseURL+"/api/generate",
		jsonReader(payload),
	)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: building generate ping: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: generate ping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GenerateResponse{}, fmt.Errorf("ollama: generate ping returned status %d", resp.StatusCode)
	}

	var gr GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: decoding generate response: %w", err)
	}

	return gr, nil
}

// jsonReader returns an io.Reader over a JSON string without importing strings
// directly (avoids an extra import for a trivial helper).
func jsonReader(s string) *jsonStringReader {
	return &jsonStringReader{s: s, pos: 0}
}

type jsonStringReader struct {
	s   string
	pos int
}

func (r *jsonStringReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.pos:])
	r.pos += n
	return n, nil
}
