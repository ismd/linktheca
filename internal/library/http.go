package library

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

type HTTP struct {
	svc *Service
}

func NewHTTP(svc *Service) *HTTP {
	return &HTTP{svc: svc}
}

// SaveHandler returns the http.HandlerFunc for POST /library.
func (h *HTTP) SaveHandler() http.HandlerFunc { return h.save }

// ListHandler returns the http.HandlerFunc for GET /library.
func (h *HTTP) ListHandler() http.HandlerFunc { return h.list }

// GetHandler returns the http.HandlerFunc for GET /library/:id.
func (h *HTTP) GetHandler() http.HandlerFunc { return h.get }

// UpdateHandler returns the http.HandlerFunc for PATCH /library/:id.
func (h *HTTP) UpdateHandler() http.HandlerFunc { return h.update }

// DeleteHandler returns the http.HandlerFunc for DELETE /library/:id.
func (h *HTTP) DeleteHandler() http.HandlerFunc { return h.delete }

func (h *HTTP) save(w http.ResponseWriter, r *http.Request) {
	var req SaveRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	if req.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "url is required")
		return
	}

	userID := coreauth.UserID(r.Context())
	item, err := h.svc.SaveURL(r.Context(), userID, req.URL)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *HTTP) list(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	params := ListParams{
		UserID: userID,
		State:  r.URL.Query().Get("state"),
		Limit:  limit,
		Offset: offset,
	}

	if fav := r.URL.Query().Get("favorite"); fav != "" {
		v := fav == "true"
		params.Favorite = &v
	}

	result, err := h.svc.List(r.Context(), params)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *HTTP) get(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	item, err := h.svc.GetByID(r.Context(), userID, itemID)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *HTTP) update(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	item, err := h.svc.Update(r.Context(), userID, itemID, req)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *HTTP) delete(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	if err := h.svc.Delete(r.Context(), userID, itemID); err != nil {
		writeLibraryError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeLibraryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "item not found")
	case errors.Is(err, ErrAlreadySaved):
		httpx.WriteError(w, http.StatusConflict, "already_saved", "article already in library")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
