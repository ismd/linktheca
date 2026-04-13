package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func (h *HTTP) Routes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
		r.Post("/refresh", h.refresh)

		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireUser(h.issuer))
			r.Post("/logout", h.logout)
			r.Get("/me", h.me)
		})
	})
}

func (h *HTTP) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Register(r.Context(), req, r.UserAgent())
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

	resp, err := h.svc.Login(r.Context(), req, r.UserAgent())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *HTTP) refresh(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
}

func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
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
