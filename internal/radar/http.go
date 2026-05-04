package radar

import (
	"encoding/json"
	"errors"
	"net/http"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

type HTTP struct {
	svc *Service
}

func NewHTTP(svc *Service) *HTTP {
	return &HTTP{svc: svc}
}

func (h *HTTP) CreateTopicHandler() http.HandlerFunc {
	return h.createTopic
}

func (h *HTTP) createTopic(w http.ResponseWriter, r *http.Request) {
	var req CreateTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	userID := coreauth.UserID(r.Context())

	topic, err := h.svc.CreateTopic(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, topic)
}

// DisabledHandler is mounted on /radar/* when LINKTHECA_RADAR_ENABLED=false.
// Returns 501 with a stable error code so the CLI can produce a useful message.
func DisabledHandler(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "radar_disabled", "radar feature is disabled on this server")
}

func writeRadarError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, ErrEmbedderUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "embedder_unavailable",
			"embedding service is unavailable, try again later")
	case errors.Is(err, ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "duplicate", "resource already exists")
	case errors.Is(err, ErrFeedNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "feed not found")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
