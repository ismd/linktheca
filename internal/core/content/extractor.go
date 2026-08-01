package content

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
)

type Article struct {
	URL             string
	CanonicalURL    string
	Title           string
	Byline          string
	Excerpt         string
	Text            string
	HTML            string
	Lang            string
	ImageURL        string
	Favicon         string
	SiteName        string
	PublishedTime   time.Time
	ModifiedTime    time.Time
	ReadingTimeSecs int
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
	req.Header.Set("User-Agent", "Linktheca/0.1")

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

	publishedTime, err := doc.PublishedTime()
	if err != nil {
		publishedTime = time.Time{}
	}

	modifiedTime, err := doc.ModifiedTime()
	if err != nil {
		modifiedTime = time.Time{}
	}

	return &Article{
		URL:             rawURL,
		CanonicalURL:    "",
		Title:           doc.Title(),
		Byline:          doc.Byline(),
		Excerpt:         doc.Excerpt(),
		Text:            text,
		HTML:            htmlBuf.String(),
		Lang:            doc.Language(),
		ImageURL:        absoluteURL(doc.ImageURL(), resp.Request.URL),
		Favicon:         doc.Favicon(),
		SiteName:        doc.SiteName(),
		PublishedTime:   publishedTime,
		ModifiedTime:    modifiedTime,
		ReadingTimeSecs: EstimateReadingTime(text),
	}, nil
}

// absoluteURL resolves ref against base. Readability rewrites the URLs inside
// the article body, but leaves metadata such as og:image exactly as the page
// wrote it, so a site declaring <meta property="og:image" content="/img.png">
// hands us a path we cannot download. An unparsable ref is returned untouched
// and fails later at download time, where the error is already reported.
func absoluteURL(ref string, base *url.URL) string {
	if ref == "" || base == nil {
		return ref
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return ref
	}

	return base.ResolveReference(parsed).String()
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
