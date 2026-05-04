package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
