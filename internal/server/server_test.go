package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/auth"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/core/media"
	"github.com/ismd/linktheca/internal/server"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFullAuthFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	cfg := &config.Config{
		HTTPAddr:            ":0",
		LogLevel:            "error",
		LogFormat:           "text",
		JWTSecret:           "integration-test-secret-that-is-long-enough",
		JWTAccessTTL:        15 * 60 * 1_000_000_000, // 15 min in ns
		JWTRefreshTTL:       24 * 60 * 60 * 1_000_000_000,
		RegistrationEnabled: true,
	}

	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logging.New(io.Discard, "text", "error"),
		DB:     pool,
	})

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// --- Register ---
	regBody, _ := json.Marshal(auth.RegisterRequest{
		Email: "e2e@example.com", Password: "a-strong-password", DisplayName: "E2E",
	})
	resp, err := http.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(regBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var reg auth.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reg))
	resp.Body.Close()
	require.True(t, reg.User.IsAdmin, "first user must be admin")

	// --- GET /auth/me ---
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var me auth.User
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	resp.Body.Close()
	require.Equal(t, "e2e@example.com", me.Email)

	// --- Refresh ---
	rfBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(rfBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var refreshed auth.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&refreshed))
	resp.Body.Close()
	require.NotEqual(t, reg.Tokens.RefreshToken, refreshed.Tokens.RefreshToken)

	// --- Old refresh must now fail ---
	oldRfBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(oldRfBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// --- Logout (with new refresh) ---
	logoutBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: refreshed.Tokens.RefreshToken})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/auth/logout", bytes.NewReader(logoutBody))
	req.Header.Set("Authorization", "Bearer "+refreshed.Tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// --- Logged-out refresh must fail ---
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(logoutBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegrationRegistrationDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	cfg := &config.Config{
		HTTPAddr:            ":0",
		JWTSecret:           "integration-test-secret-that-is-long-enough",
		JWTAccessTTL:        15 * 60 * 1_000_000_000,
		JWTRefreshTTL:       24 * 60 * 60 * 1_000_000_000,
		RegistrationEnabled: false,
	}
	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logging.New(io.Discard, "text", "error"),
		DB:     pool,
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "closed@example.com", Password: "a-strong-password", DisplayName: "X",
	})
	resp, err := http.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

func TestRadarDisabled_Returns403OnAnyRoute(t *testing.T) {
	cfg := &config.Config{
		HTTPAddr:     ":0",
		JWTSecret:    "test-secret-at-least-32-bytes-long-for-hmac",
		JWTAccessTTL: 15 * time.Minute,
		RadarEnabled: false,
	}
	deps := server.Deps{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     nil, // not used by /radar disabled handler
		Radar:  nil,
	}
	srv := server.New(deps)
	defer srv.Close()

	for _, path := range []string{"/radar/topics", "/radar/feeds", "/radar/subscriptions", "/radar/anything"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code, "path %s", path)
		require.Contains(t, rec.Body.String(), "radar_disabled")
	}
}

// newMediaServer builds a server serving downloaded images out of mediaDir.
func newMediaServer(t *testing.T, mediaDir string) *http.Server {
	t.Helper()

	srv := server.New(server.Deps{
		Config: &config.Config{
			HTTPAddr:     ":0",
			JWTSecret:    "test-secret-at-least-32-bytes-long-for-hmac",
			JWTAccessTTL: 15 * time.Minute,
			MediaDir:     mediaDir,
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     nil, // not used when serving files off disk
	})
	t.Cleanup(func() { srv.Close() })

	return srv
}

func TestMediaImageIsServedFromDisk(t *testing.T) {
	mediaDir := t.TempDir()
	require.NoError(t, os.MkdirAll(media.ImagesDir(mediaDir), 0o755))

	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte("pixels")...)
	require.NoError(t, os.WriteFile(filepath.Join(media.ImagesDir(mediaDir), "a1b2c3.png"), png, 0o644))

	srv := newMediaServer(t, mediaDir)

	// Previews are public — no Authorization header here
	req := httptest.NewRequest(http.MethodGet, "/media/images/a1b2c3.png", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, png, rec.Body.Bytes())
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
}

func TestMediaImageMissingReturns404(t *testing.T) {
	srv := newMediaServer(t, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/media/images/nope.png", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	// A missing file must not be cached as if it were an immutable asset
	require.Empty(t, rec.Header().Get("Cache-Control"))
}

func TestMediaImagesAreNotBrowsable(t *testing.T) {
	mediaDir := t.TempDir()
	require.NoError(t, os.MkdirAll(media.ImagesDir(mediaDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(media.ImagesDir(mediaDir), "secret.png"), []byte("x"), 0o644))

	srv := newMediaServer(t, mediaDir)

	// Listing the directory would leak every saved article's preview
	for _, path := range []string{"/media/images/", "/media/images", "/media/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		require.NotEqual(t, http.StatusOK, rec.Code, "path %s", path)
		require.NotContains(t, rec.Body.String(), "secret.png", "path %s", path)
	}
}

func TestMediaImageRejectsTraversal(t *testing.T) {
	mediaDir := t.TempDir()
	require.NoError(t, os.MkdirAll(media.ImagesDir(mediaDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mediaDir, "outside.txt"), []byte("secret"), 0o644))

	srv := newMediaServer(t, mediaDir)

	for _, path := range []string{"/media/images/../outside.txt", "/media/images/..%2foutside.txt"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		require.NotEqual(t, http.StatusOK, rec.Code, "path %s", path)
		require.NotContains(t, rec.Body.String(), "secret", "path %s", path)
	}
}
