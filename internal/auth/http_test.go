package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-at-least-32-bytes-long-for-hmac"

func newTestHTTP(t *testing.T, registration bool) (*chi.Mux, *auth.Service, *mockStore) {
	t.Helper()
	store := newMockStore()
	issuer := coreauth.NewJWTIssuer(testSecret, 15*time.Minute)
	svc := auth.NewService(store, issuer, auth.ServiceConfig{
		RefreshTTL:          720 * time.Hour,
		RegistrationEnabled: registration,
	})
	h := auth.NewHTTP(svc, issuer)

	r := chi.NewRouter()
	r.Post("/auth/register", h.RegisterHandler())
	r.Post("/auth/login", h.LoginHandler())
	r.Post("/auth/refresh", h.RefreshHandler())
	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", h.LogoutHandler())
		r.Get("/auth/me", h.MeHandler())
	})
	return r, svc, store
}

func TestHTTPRegisterSuccess(t *testing.T) {
	router, _, _ := newTestHTTP(t, true)

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "http@example.com", Password: "a-strong-password", DisplayName: "HTTP",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp auth.AuthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "http@example.com", resp.User.Email)
	require.True(t, resp.User.IsAdmin)
	require.NotEmpty(t, resp.Tokens.AccessToken)
}

func TestHTTPRegisterDisabledReturns403(t *testing.T) {
	router, _, _ := newTestHTTP(t, false)

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "x@example.com", Password: "a-strong-password", DisplayName: "X",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHTTPLoginSuccess(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "li@example.com", Password: "a-strong-password", DisplayName: "LI",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(auth.LoginRequest{
		Email: "li@example.com", Password: "a-strong-password",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTPLoginWrongPassword(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "lw@example.com", Password: "a-strong-password", DisplayName: "LW",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(auth.LoginRequest{
		Email: "lw@example.com", Password: "wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHTTPRefreshRotates(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "rfa@example.com", Password: "a-strong-password", DisplayName: "RFA",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp auth.AuthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotEqual(t, reg.Tokens.RefreshToken, resp.Tokens.RefreshToken)
}

func TestHTTPLogout(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "lgo@example.com", Password: "a-strong-password", DisplayName: "LGO",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHTTPMeReturnsUser(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "meh@example.com", Password: "a-strong-password", DisplayName: "MEH",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got auth.User
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "meh@example.com", got.Email)
}

func TestHTTPMeRejectsUnauthenticated(t *testing.T) {
	router, _, _ := newTestHTTP(t, true)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
