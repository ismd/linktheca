package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestRequireUserAcceptsValidBearer(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(42, false)
	require.NoError(t, err)

	var capturedUserID int64
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = auth.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), capturedUserID)
}

func TestRequireUserRejectsMissingHeader(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireUserRejectsBadBearer(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAdminRejectsNonAdmin(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(1, false) // not admin
	require.NoError(t, err)

	handler := auth.RequireUser(issuer)(
		auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not run")
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireAdminAcceptsAdmin(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(1, true) // admin
	require.NoError(t, err)

	handler := auth.RequireUser(issuer)(
		auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

// Unused import guard
var _ = strings.TrimSpace
