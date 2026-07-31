package media_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ismd/linktheca/internal/core/media"
	"github.com/stretchr/testify/require"
)

// pngBody is a byte slice whose first bytes are the PNG signature, which is all
// http.DetectContentType looks at.
var pngBody = append([]byte("\x89PNG\r\n\x1a\n"), []byte("payload bytes")...)

func TestFetchWritesUnderConfiguredDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(pngBody)
	}))
	defer srv.Close()

	dir := t.TempDir()
	fetcher := media.NewFetcher(dir)

	img, err := fetcher.Fetch(context.Background(), srv.URL+"/preview.png")
	require.NoError(t, err)
	require.NotEmpty(t, img.Filename)

	// The returned name is relative to the images subdirectory of the configured dir
	saved := filepath.Join(dir, "images", img.Filename)
	body, err := os.ReadFile(saved)
	require.NoError(t, err)
	require.Equal(t, pngBody, body)

	// Nothing is written outside the configured dir
	require.NoDirExists(t, "media")
}

func TestFetchRejectsNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>not an image</body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	fetcher := media.NewFetcher(dir)

	_, err := fetcher.Fetch(context.Background(), srv.URL+"/preview.png")
	require.Error(t, err)

	// A rejected download leaves no leftovers behind
	entries, err := os.ReadDir(filepath.Join(dir, "images"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestFetchRejectsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fetcher := media.NewFetcher(t.TempDir())

	_, err := fetcher.Fetch(context.Background(), srv.URL+"/gone.png")
	require.Error(t, err)
}
