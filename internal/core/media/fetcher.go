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
const dir = "media/images"

type Image struct {
	Filename string
}

type Fetcher interface {
	Fetch(ctx context.Context, imageURL string) (*Image, error)
}

type httpFetcher struct {
	client *http.Client
}

func NewFetcher() Fetcher {
	return &httpFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (f *httpFetcher) Fetch(ctx context.Context, imageURL string) (*Image, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
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

	image, err := saveFile(ext, br)
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	return image, nil
}

func saveFile(ext string, br *bufio.Reader) (*Image, error) {
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

	// Rename file
	finalName := strings.TrimSuffix(file.Name(), ".tmp")
	err = os.Rename(file.Name(), finalName)
	if err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}

	ok = true
	return &Image{
		Filename: filepath.Base(finalName),
	}, nil
}
