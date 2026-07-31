package media

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var allowedContentTypes = []string{
	"image/png",
	"image/jpeg",
	"image/webp",
	"image/x-icon",
	"image/vnd.microsoft.icon",
	"image/gif",
}

const maxImageSize = 10 * 1024 * 1024 // 10 MB

type Image struct {
	Filename string
}

type Fetcher interface {
	Fetch(ctx context.Context, imageURL string) (*Image, error)
	FetchFavicon(ctx context.Context, host, faviconURL string) (*Image, error)
}

type httpFetcher struct {
	client      *http.Client
	imagesDir   string
	faviconsDir string
}

// NewFetcher returns a Fetcher saving downloads under mediaDir.
func NewFetcher(mediaDir string) Fetcher {
	return &httpFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		imagesDir:   ImagesDir(mediaDir),
		faviconsDir: FaviconsDir(mediaDir),
	}
}

// ImagesDir is where downloaded images live under a media directory. The HTTP
// layer serves this directory, so it resolves the path the same way.
func ImagesDir(mediaDir string) string {
	return filepath.Join(mediaDir, "images")
}

// FaviconsDir is where downloaded favicons live under a media directory.
func FaviconsDir(mediaDir string) string {
	return filepath.Join(mediaDir, "favicons")
}

func (f *httpFetcher) Fetch(ctx context.Context, imageURL string) (*Image, error) {
	return f.fetch(ctx, f.imagesDir, imageURL, "")
}

// FetchFavicon downloads a site's favicon and stores it under the host, so
// every article from that site shares one file. A favicon already on disk is
// reused without touching the network — that is the point of keying by host
// rather than by the URL each page happens to declare.
func (f *httpFetcher) FetchFavicon(ctx context.Context, host, faviconURL string) (*Image, error) {
	if err := checkHostKey(host); err != nil {
		return nil, err
	}

	if name, ok := storedFavicon(f.faviconsDir, host); ok {
		return &Image{Filename: name}, nil
	}

	return f.fetch(ctx, f.faviconsDir, faviconURL, host)
}

// checkHostKey guards the host before it becomes a path element.
func checkHostKey(host string) error {
	if host == "" || host == "." || host == ".." ||
		strings.ContainsAny(host, `/\`) || host != filepath.Base(host) {
		return fmt.Errorf("favicon: unusable host %q", host)
	}

	return nil
}

// storedFavicon looks for an already downloaded favicon for host, whatever
// extension it ended up with.
func storedFavicon(dir, host string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.TrimSuffix(name, filepath.Ext(name)) == host {
			return name, true
		}
	}

	return "", false
}

// fetch downloads rawURL into dir. An empty baseName picks a random file name,
// otherwise the file is stored as baseName plus the detected extension.
func (f *httpFetcher) fetch(ctx context.Context, dir, rawURL, baseName string) (*Image, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Linktheca/0.1")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxImageSize+1)
	br := bufio.NewReader(limited)
	b, err := br.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("peek: %w", err)
	}

	ct := http.DetectContentType(b)
	if !slices.Contains(allowedContentTypes, ct) {
		return nil, errors.New("mime: unsupported content type")
	}

	exts, err := mime.ExtensionsByType(ct)
	if err != nil {
		return nil, fmt.Errorf("mime: %w", err)
	}

	if len(exts) < 1 {
		return nil, errors.New("mime: cannot get extension by content type")
	}

	ext := exts[0]

	image, err := saveFile(dir, baseName, ext, br)
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	return image, nil
}

func saveFile(dir, baseName, ext string, br *bufio.Reader) (*Image, error) {
	file, err := os.CreateTemp(dir, "*"+ext+".tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	ok := false
	defer func() {
		file.Close()

		if !ok {
			os.Remove(file.Name())
		}
	}()

	n, err := io.Copy(file, br)
	if err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}

	if n > maxImageSize {
		return nil, errors.New("file size exceeds limit")
	}

	err = file.Close()
	if err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}

	// Rename file. A caller-supplied base name replaces the random one; the
	// rename stays atomic, so a concurrent download of the same key is harmless.
	finalName := strings.TrimSuffix(file.Name(), ".tmp")
	if baseName != "" {
		finalName = filepath.Join(dir, baseName+ext)
	}

	err = os.Rename(file.Name(), finalName)
	if err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}

	ok = true
	return &Image{
		Filename: filepath.Base(finalName),
	}, nil
}
