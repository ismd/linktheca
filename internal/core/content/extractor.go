package content

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

type Article struct {
	URL              string
	CanonicalURL     string
	Title            string
	Byline           string
	Excerpt          string
	Text             string
	HTML             string
	Lang             string
	ReadingTimeSecs  int
}

type Extractor interface {
	Extract(ctx context.Context, url string) (*Article, error)
}

type readabilityExtractor struct {
	client *http.Client
}

func NewExtractor() Extractor {
	return &readabilityExtractor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *readabilityExtractor) Extract(ctx context.Context, rawURL string) (*Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Linktheca/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: status %d", resp.StatusCode)
	}

	doc, err := readability.FromReader(resp.Body, resp.Request.URL)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var textBuf, htmlBuf strings.Builder
	if doc.Node != nil {
		if err := doc.RenderText(&textBuf); err != nil {
			return nil, fmt.Errorf("render text: %w", err)
		}

		if err := doc.RenderHTML(&htmlBuf); err != nil {
			return nil, fmt.Errorf("render html: %w", err)
		}
	}
	text := textBuf.String()

	return &Article{
		URL:             rawURL,
		CanonicalURL:    "",
		Title:           doc.Title(),
		Byline:          doc.Byline(),
		Excerpt:         doc.Excerpt(),
		Text:            text,
		HTML:            htmlBuf.String(),
		Lang:            doc.Language(),
		ReadingTimeSecs: EstimateReadingTime(text),
	}, nil
}

// EstimateReadingTime returns estimated reading time in seconds
// Average reading speed: ~200 words per minute
func EstimateReadingTime(text string) int {
	words := len(strings.Fields(text))
	secs := (words * 60) / 200

	if secs == 0 {
		secs = 1
	}

	return secs
}
