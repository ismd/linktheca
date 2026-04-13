package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismd/linktheca/internal/auth"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/logging"
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
