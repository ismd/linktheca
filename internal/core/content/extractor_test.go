package content_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/netsafe"
	"github.com/stretchr/testify/require"
)

// loopbackClient lets the extractor reach httptest servers, which listen on
// loopback, an address the production client refuses.
func loopbackClient() *http.Client {
	return netsafe.NewClient(netsafe.Options{Timeout: 5 * time.Second, AllowPrivate: true})
}

const testHTML = `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
<article>
<h1>Test Article</h1>
<p>By John Doe</p>
<p>This is the first paragraph of a test article. It contains enough text
to be recognized as real content by the readability algorithm. We need
several sentences to make this work properly.</p>
<p>This is the second paragraph. It also contains meaningful content that
should be extracted by the readability parser. The more text we have here,
the better the extraction will work.</p>
<p>And a third paragraph for good measure. Content extraction algorithms
typically need a reasonable amount of text to distinguish article content
from boilerplate navigation and footer elements.</p>
</article>
</body>
</html>`

func TestExtractFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testHTML))
	}))
	defer srv.Close()

	ext := content.NewExtractor(loopbackClient())
	result, err := ext.Extract(context.Background(), srv.URL)

	require.NoError(t, err)
	require.Equal(t, srv.URL, result.URL)
	require.Contains(t, result.Title, "Test Article")
	require.NotEmpty(t, result.Text)
	require.NotEmpty(t, result.HTML)
}

// articleHTMLWithImage builds a page whose og:image holds the given, possibly
// relative, URL. Readability leaves metadata URLs untouched, so the extractor
// has to resolve them against the page itself.
func articleHTMLWithImage(image string) string {
	return strings.Replace(testHTML,
		"<head><title>Test Article</title></head>",
		`<head><title>Test Article</title>`+
			`<meta property="og:image" content="`+image+`"></head>`, 1)
}

func TestExtractResolvesImageURL(t *testing.T) {
	tests := map[string]struct {
		image string
		want  func(base string) string
	}{
		"root relative": {
			image: "/uploads/preview.svg",
			want:  func(base string) string { return base + "/uploads/preview.svg" },
		},
		"path relative": {
			image: "preview.png",
			want:  func(base string) string { return base + "/article/preview.png" },
		},
		"protocol relative": {
			image: "//cdn.example.com/preview.png",
			want:  func(string) string { return "http://cdn.example.com/preview.png" },
		},
		"already absolute": {
			image: "https://cdn.example.com/preview.png",
			want:  func(string) string { return "https://cdn.example.com/preview.png" },
		},
		"empty": {
			image: "",
			want:  func(string) string { return "" },
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(articleHTMLWithImage(tt.image)))
			}))
			defer srv.Close()

			ext := content.NewExtractor(loopbackClient())
			result, err := ext.Extract(context.Background(), srv.URL+"/article/post")

			require.NoError(t, err)
			require.Equal(t, tt.want(srv.URL), result.ImageURL)
		})
	}
}

func TestExtractFromURLFetchError(t *testing.T) {
	ext := content.NewExtractor(loopbackClient())
	_, err := ext.Extract(context.Background(), "http://127.0.0.1:1/nonexistent")

	var fe *content.FetchError
	require.ErrorAs(t, err, &fe)
	require.Zero(t, fe.Status)
	require.True(t, content.IsTransient(err), "a refused connection may work later")
}

func TestExtractRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testHTML))
	}))
	defer srv.Close()

	ext := content.NewExtractor(netsafe.NewClient(netsafe.Options{Timeout: 5 * time.Second}))
	_, err := ext.Extract(context.Background(), srv.URL)

	require.ErrorIs(t, err, netsafe.ErrBlockedAddress)
	require.False(t, content.IsTransient(err))
}

func TestExtractClassifiesHTTPFailures(t *testing.T) {
	tests := map[string]struct {
		status      int
		contentType string
		transient   bool
		message     string
	}{
		"not found":         {status: http.StatusNotFound, transient: false, message: "fetch: status 404"},
		"gone":              {status: http.StatusGone, transient: false, message: "fetch: status 410"},
		"forbidden":         {status: http.StatusForbidden, transient: false, message: "fetch: status 403"},
		"rate limited":      {status: http.StatusTooManyRequests, transient: true, message: "fetch: status 429"},
		"request timeout":   {status: http.StatusRequestTimeout, transient: true, message: "fetch: status 408"},
		"server error":      {status: http.StatusInternalServerError, transient: true, message: "fetch: status 500"},
		"unavailable":       {status: http.StatusServiceUnavailable, transient: true, message: "fetch: status 503"},
		"pdf":               {status: http.StatusOK, contentType: "application/pdf", message: "fetch: not an HTML page (application/pdf)"},
		"json":              {status: http.StatusOK, contentType: "application/json; charset=utf-8", message: "fetch: not an HTML page (application/json)"},
		"garbage mime type": {status: http.StatusOK, contentType: "nonsense;;", message: "fetch: not an HTML page (nonsense;;)"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(testHTML))
			}))
			defer srv.Close()

			_, err := content.NewExtractor(loopbackClient()).Extract(context.Background(), srv.URL)

			var fe *content.FetchError
			require.ErrorAs(t, err, &fe)
			require.Equal(t, tt.status, fe.Status)
			require.Equal(t, tt.transient, content.IsTransient(err))
			require.Equal(t, tt.message, err.Error())
		})
	}
}

func TestExtractAcceptsHTMLContentTypes(t *testing.T) {
	for _, ct := range []string{"", "text/html", "TEXT/HTML; charset=windows-1251", "application/xhtml+xml"} {
		t.Run(ct, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				// An empty value still has to reach the client as "no header":
				// net/http would otherwise sniff one from the body.
				w.Header()["Content-Type"] = nil
				if ct != "" {
					w.Header().Set("Content-Type", ct)
				}
				_, _ = w.Write([]byte(testHTML))
			}))
			defer srv.Close()

			result, err := content.NewExtractor(loopbackClient()).Extract(context.Background(), srv.URL)

			require.NoError(t, err)
			require.Contains(t, result.Title, "Test Article")
		})
	}
}

// Past the size cap the page is cut off rather than rejected: the article
// near the top still comes through.
func TestExtractReadsOnlyTheStartOfHugePages(t *testing.T) {
	content.SetMaxPageBytes(t, 64<<10)
	padding := "<!-- " + strings.Repeat("x", 1<<20) + " -->"
	tail := "<p>This paragraph sits past the cap, after a megabyte of comment, and must not be read.</p>"
	page := strings.Replace(testHTML, "</article>", padding+tail+"</article>", 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	result, err := content.NewExtractor(loopbackClient()).Extract(context.Background(), srv.URL)

	require.NoError(t, err)
	require.Contains(t, result.Text, "first paragraph of a test article")
	require.NotContains(t, result.Text, "past the cap")
}

func TestExtractEmptyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}))
	defer srv.Close()

	result, err := content.NewExtractor(loopbackClient()).Extract(context.Background(), srv.URL)

	require.NoError(t, err)
	require.Empty(t, result.Text)
	require.Empty(t, result.Excerpt)
}

func TestIsTransientIgnoresOtherErrors(t *testing.T) {
	require.False(t, content.IsTransient(nil))
	require.False(t, content.IsTransient(errors.New("parse: broken")))
}

func TestReadingTimeEstimation(t *testing.T) {
	require.Equal(t, 1, content.EstimateReadingTime(""))
	require.Equal(t, 1, content.EstimateReadingTime("short text"))

	// ~200 words → should be about 1 minute (200 WPM)
	long := strings.Repeat("word ", 200)
	got := content.EstimateReadingTime(long)
	require.GreaterOrEqual(t, got, 55)
	require.LessOrEqual(t, got, 65)
}
