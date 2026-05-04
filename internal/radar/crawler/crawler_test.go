package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/stretchr/testify/require"
)

func TestHTTPFetcher_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", "Wed, 22 Apr 2026 12:00:00 GMT")
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	got, err := f.Fetch(context.Background(), srv.URL, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, got.StatusCode)
	require.Equal(t, `"abc"`, got.Etag)
	require.Equal(t, "Wed, 22 Apr 2026 12:00:00 GMT", got.LastModified)
	require.Contains(t, string(got.Body), "rss")
	require.False(t, got.NotModified)
}

func TestHTTPFetcher_304(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, `"abc"`, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	got, err := f.Fetch(context.Background(), srv.URL, `"abc"`, "")
	require.NoError(t, err)
	require.True(t, got.NotModified)
}

func TestHTTPFetcher_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	_, err := f.Fetch(context.Background(), srv.URL, "", "")
	require.Error(t, err)
}

func TestParse_RSS(t *testing.T) {
	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>HN</title>
<item>
  <title>OpenAI ships</title>
  <link>https://news.example/post/1</link>
  <description>About models</description>
  <guid>hn:1</guid>
  <pubDate>Wed, 22 Apr 2026 12:00:00 GMT</pubDate>
</item>
</channel></rss>`)

	got, err := crawler.Parse(rss)
	require.NoError(t, err)
	require.Len(t, got, 1)

	upserts := crawler.ToUpserts(42, got)
	require.Equal(t, int64(42), upserts[0].FeedID)
	require.Equal(t, "https://news.example/post/1", upserts[0].URL)
	require.NotNil(t, upserts[0].ExternalID)
	require.Equal(t, "hn:1", *upserts[0].ExternalID)
	require.NotNil(t, upserts[0].Title)
	require.Equal(t, "OpenAI ships", *upserts[0].Title)
}

func TestParse_GarbageReturnsError(t *testing.T) {
	_, err := crawler.Parse([]byte("not xml"))
	require.Error(t, err)
}

// Compile-time check that ToUpserts returns []radar.FindingUpsert.
var _ []radar.FindingUpsert = crawler.ToUpserts(0, nil)
