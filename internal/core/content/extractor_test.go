package content_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/stretchr/testify/require"
)

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

	ext := content.NewExtractor()
	result, err := ext.Extract(context.Background(), srv.URL)

	require.NoError(t, err)
	require.Equal(t, srv.URL, result.URL)
	require.Contains(t, result.Title, "Test Article")
	require.NotEmpty(t, result.Text)
	require.NotEmpty(t, result.HTML)
}

func TestExtractFromURLFetchError(t *testing.T) {
	ext := content.NewExtractor()
	_, err := ext.Extract(context.Background(), "http://127.0.0.1:1/nonexistent")

	require.Error(t, err)
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
