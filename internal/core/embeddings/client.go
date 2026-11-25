// Package embeddings provides a text-to-vector client interface with a TEI
// (HuggingFace Text Embeddings Inference) implementation and a deterministic
// fake for tests.
package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type TEIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewTEIClient(baseURL string, timeout time.Duration) *TEIClient {
	return &TEIClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *TEIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	// Truncate rather than reject inputs longer than the model takes: a
	// finding is embedded from the start of its article, and that start is
	// all the model can see anyway.
	body, err := json.Marshal(map[string]any{"inputs": text, "truncate": true})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tei status %d: %s", resp.StatusCode, string(snippet))
	}

	var out [][]float32

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("tei returned empty embedding list")
	}

	return out[0], nil
}
