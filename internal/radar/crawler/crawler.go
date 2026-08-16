// Package crawler turns RSS/Atom feed URLs into structured items. Fetcher
// abstracts the HTTP layer so jobs can be tested without real network IO.
package crawler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
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

// ParsedFeed is what one fetched document yields: the channel's own title and
// its entries.
type ParsedFeed struct {
	Title string
	Items []*gofeed.Item
}

func Parse(body []byte) (*ParsedFeed, error) {
	parser := gofeed.NewParser()

	feed, err := parser.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}

	return &ParsedFeed{Title: strings.TrimSpace(feed.Title), Items: feed.Items}, nil
}

func ToUpserts(feedID int64, items []*gofeed.Item) []radar.FindingUpsert {
	out := make([]radar.FindingUpsert, 0, len(items))
	for _, it := range items {
		if it == nil || strings.TrimSpace(it.Link) == "" {
			continue
		}

		up := radar.FindingUpsert{FeedID: feedID, URL: strings.TrimSpace(it.Link)}

		ext := strings.TrimSpace(it.GUID)
		if ext == "" {
			ext = up.URL
		}

		up.ExternalID = &ext

		if t := strings.TrimSpace(it.Title); t != "" {
			up.Title = &t
		}

		if d := sanitizeSummary(it.Description); d != "" {
			up.Summary = &d
		}

		if it.PublishedParsed != nil {
			pp := *it.PublishedParsed
			up.PublishedAt = &pp
		}

		out = append(out, up)
	}

	return out
}

// Some aggregator feeds emit a "Comments" anchor or a bare URL as <description>.
// That text adds no signal to embeddings and uniformly biases vectors across
// all findings from the same feed.
var summaryPlaceholders = map[string]struct{}{
	"comment":    {},
	"comments":   {},
	"discuss":    {},
	"discussion": {},
	"link":       {},
	"read":       {},
	"read more":  {},
	"article":    {},
}

func sanitizeSummary(raw string) string {
	text := strings.TrimSpace(plainText(raw))
	if text == "" {
		return ""
	}

	if _, ok := summaryPlaceholders[strings.ToLower(text)]; ok {
		return ""
	}

	if isBareURL(text) {
		return ""
	}

	return text
}

func plainText(raw string) string {
	z := html.NewTokenizer(strings.NewReader(raw))

	var b strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			b.Write(z.Text())
		}
	}
}

func isBareURL(s string) bool {
	if strings.ContainsAny(s, " \t\n\r") {
		return false
	}

	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
