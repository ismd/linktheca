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

	r := chi.NewRouter()
	h := auth.NewHTTP(svc, issuer)
	h.Routes(r)
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
	}, "ua")
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
	}, "ua")
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
