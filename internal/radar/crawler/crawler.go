// Package crawler turns RSS/Atom feed URLs into structured items. Fetcher
// abstracts the HTTP layer so jobs can be tested without real network IO.
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FetchResult struct {
	StatusCode   int
	Body         []byte
	Etag         string
	LastModified string
	NotModified  bool
}

type Fetcher interface {
	Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error)
}

type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{Timeout: timeout}}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", "linktheca/0.1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{
			StatusCode:   resp.StatusCode,
			Etag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			NotModified:  true,
		}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MiB cap
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &FetchResult{
		StatusCode:   resp.StatusCode,
		Body:         body,
		Etag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}
