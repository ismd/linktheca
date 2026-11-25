package content

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/ismd/linktheca/internal/core/netsafe"
)

// maxPageBytes caps how much of a page is read. Pages past it are mostly
// inlined scripts and styles; the article itself is parsed from what fits.
// A variable only so tests can lower it.
var maxPageBytes int64 = 5 << 20

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

// FetchError reports a page that could not be downloaded. Transient marks
// failures worth retrying later: timeouts, dropped connections, 5xx, 429.
// Everything else, such as a 404, a PDF or an address that is not allowed,
// would fail the same way again.
type FetchError struct {
	// Status is the HTTP status, or 0 when no response arrived.
	Status    int
	Transient bool
	Err       error
}

func (e *FetchError) Error() string { return "fetch: " + e.Err.Error() }

func (e *FetchError) Unwrap() error { return e.Err }

// IsTransient reports whether err is a fetch failure worth retrying.
func IsTransient(err error) bool {
	var fe *FetchError

	return errors.As(err, &fe) && fe.Transient
}

type readabilityExtractor struct {
	client *http.Client
}

// NewExtractor returns an Extractor that downloads pages with client. Pass a
// netsafe client: the URLs come from users and feeds.
func NewExtractor(client *http.Client) Extractor {
	return &readabilityExtractor{client: client}
}

func (e *readabilityExtractor) Extract(ctx context.Context, rawURL string) (*Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Linktheca/0.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, &FetchError{Transient: isTransientNetError(err), Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &FetchError{
			Status:    resp.StatusCode,
			Transient: isTransientStatus(resp.StatusCode),
			Err:       fmt.Errorf("status %d", resp.StatusCode),
		}
	}

	if mt, ok := htmlMediaType(resp.Header.Get("Content-Type")); !ok {
		return nil, &FetchError{
			Status: resp.StatusCode,
			Err:    fmt.Errorf("not an HTML page (%s)", mt),
		}
	}

	doc, err := readability.FromReader(io.LimitReader(resp.Body, maxPageBytes), resp.Request.URL)
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

func isTransientStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	}

	return status >= 500
}

// isTransientNetError tells network hiccups from failures that a retry would
// repeat: an address netsafe refuses, a redirect loop, a domain that does not
// exist.
func isTransientNetError(err error) bool {
	if errors.Is(err, netsafe.ErrBlockedAddress) || errors.Is(err, netsafe.ErrTooManyRedirects) {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}

	return true
}

// htmlMediaType reports whether a Content-Type header describes an HTML page,
// returning the media type for the error message when it does not. A missing
// header is given the benefit of the doubt.
func htmlMediaType(header string) (string, bool) {
	if header == "" {
		return "", true
	}

	mt, _, err := mime.ParseMediaType(header)
	if err != nil {
		return header, false
	}

	return mt, mt == "text/html" || mt == "application/xhtml+xml"
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
