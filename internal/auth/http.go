package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

type HTTP struct {
	svc    *Service
	issuer *coreauth.JWTIssuer
}

func NewHTTP(svc *Service, issuer *coreauth.JWTIssuer) *HTTP {
	return &HTTP{svc: svc, issuer: issuer}
}

// RegisterHandler returns the http.HandlerFunc for POST /auth/register.
func (h *HTTP) RegisterHandler() http.HandlerFunc { return h.register }

// LoginHandler returns the http.HandlerFunc for POST /auth/login.
func (h *HTTP) LoginHandler() http.HandlerFunc { return h.login }

// RefreshHandler returns the http.HandlerFunc for POST /auth/refresh.
func (h *HTTP) RefreshHandler() http.HandlerFunc { return h.refresh }

// LogoutHandler returns the http.HandlerFunc for POST /auth/logout.
func (h *HTTP) LogoutHandler() http.HandlerFunc { return h.logout }

// MeHandler returns the http.HandlerFunc for GET /auth/me.
func (h *HTTP) MeHandler() http.HandlerFunc { return h.me }

func (h *HTTP) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Register(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *HTTP) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Login(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *HTTP) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Refresh(r.Context(), req)

	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	if err := h.svc.Logout(r.Context(), req); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	user, err := h.svc.Me(r.Context(), userID)

	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, user)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRegistrationDisabled):
		httpx.WriteError(w, http.StatusForbidden, "registration_disabled", "registration is closed")
	case errors.Is(err, ErrWeakPassword):
		httpx.WriteError(w, http.StatusBadRequest, "weak_password", "password is too short")
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "email already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
