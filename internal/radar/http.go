package radar

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

func (h *HTTP) PreviewTopicHandler() http.HandlerFunc { return h.previewTopic }

func (h *HTTP) previewTopic(w http.ResponseWriter, r *http.Request) {
	var req PreviewTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	userID := coreauth.UserID(r.Context())

	preview, err := h.svc.PreviewTopic(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, preview)
}

// DisabledHandler is mounted on /radar/* when LINKTHECA_RADAR_ENABLED=false.
// Returns 403 (the feature is implemented but administratively disabled, not
// unimplemented) with a stable error code so clients can show a useful message.
func DisabledHandler(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusForbidden, "radar_disabled", "radar feature is disabled on this server")
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
	case errors.Is(err, ErrQuotaExceeded):
		httpx.WriteError(w, http.StatusConflict, "quota_exceeded", err.Error())
	case errors.Is(err, ErrFeedNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "feed not found")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}

func (h *HTTP) SubscribeHandler() http.HandlerFunc { return h.subscribe }

// AddUserFeedHandler returns the http.HandlerFunc for POST /radar/feeds:
// the caller's own personal feed.
func (h *HTTP) AddUserFeedHandler() http.HandlerFunc { return h.addUserFeed }

func (h *HTTP) addUserFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	userID := coreauth.UserID(r.Context())

	res, err := h.svc.AddUserFeed(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	status := http.StatusOK // an existing catalog feed was reused
	if res.Created {
		status = http.StatusCreated
	}

	httpx.WriteJSON(w, status, res)
}

// AddGlobalFeedHandler returns the http.HandlerFunc for
// POST /admin/radar/feeds (admin).
func (h *HTTP) AddGlobalFeedHandler() http.HandlerFunc { return h.addGlobalFeed }

func (h *HTTP) addGlobalFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	feed, err := h.svc.AddGlobalFeed(r.Context(), req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, feed)
}

func (h *HTTP) subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	userID := coreauth.UserID(r.Context())

	sub, err := h.svc.Subscribe(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, sub)
}

// ListTopicsHandler returns the http.HandlerFunc for GET /radar/topics.
func (h *HTTP) ListTopicsHandler() http.HandlerFunc { return h.listTopics }

// GetTopicHandler returns the http.HandlerFunc for GET /radar/topics/{id}.
func (h *HTTP) GetTopicHandler() http.HandlerFunc { return h.getTopic }

// UpdateTopicHandler returns the http.HandlerFunc for PATCH /radar/topics/{id}.
func (h *HTTP) UpdateTopicHandler() http.HandlerFunc { return h.updateTopic }

// DeleteTopicHandler returns the http.HandlerFunc for DELETE /radar/topics/{id}.
func (h *HTTP) DeleteTopicHandler() http.HandlerFunc { return h.deleteTopic }

func parseRadarID(r *http.Request) (int64, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func (h *HTTP) listTopics(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	items, err := h.svc.ListTopics(r.Context(), userID)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *HTTP) getTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	topic, err := h.svc.GetTopic(r.Context(), userID, id)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, topic)
}

func (h *HTTP) updateTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req UpdateTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	topic, err := h.svc.UpdateTopic(r.Context(), userID, id, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, topic)
}

func (h *HTTP) deleteTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.svc.DeleteTopic(r.Context(), userID, id); err != nil {
		writeRadarError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListMatchesHandler returns the http.HandlerFunc for GET /radar/matches.
func (h *HTTP) ListMatchesHandler() http.HandlerFunc { return h.listMatches }

// GetMatchHandler returns the http.HandlerFunc for GET /radar/matches/{id}.
func (h *HTTP) GetMatchHandler() http.HandlerFunc { return h.getMatch }

func (h *HTTP) getMatch(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	mv, err := h.svc.GetMatch(r.Context(), userID, id)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mv)
}

// UpdateMatchHandler returns the http.HandlerFunc for PATCH /radar/matches/{id}.
func (h *HTTP) UpdateMatchHandler() http.HandlerFunc { return h.updateMatch }

// StatusHandler returns the http.HandlerFunc for GET /radar/status.
func (h *HTTP) StatusHandler() http.HandlerFunc { return h.status }

// ListFeedsHandler returns the http.HandlerFunc for GET /radar/feeds: the
// shared catalog plus the caller's own personal feeds.
func (h *HTTP) ListFeedsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.listFeedsScoped(w, r, FeedScopeVisible)
	}
}

// ListGlobalFeedsHandler returns the http.HandlerFunc for
// GET /admin/radar/feeds (admin): the shared catalog alone.
func (h *HTTP) ListGlobalFeedsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.listFeedsScoped(w, r, FeedScopeGlobal)
	}
}

func (h *HTTP) listMatches(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	q := r.URL.Query()

	params := ListMatchesParams{UserID: userID}

	if topicStr := q.Get("topic_id"); topicStr != "" {
		topicID, err := strconv.ParseInt(topicStr, 10, 64)
		if err != nil || topicID <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid topic_id")
			return
		}
		params.TopicID = &topicID
	}

	if state := q.Get("state"); state != "" {
		if state != "new" && state != "seen" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "state must be new|seen")
			return
		}
		params.State = &state
	}

	if l, err := strconv.Atoi(q.Get("limit")); err == nil {
		params.Limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil {
		params.Offset = o
	}

	result, err := h.svc.ListMatches(r.Context(), params)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *HTTP) updateMatch(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req UpdateMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	if err := h.svc.SetMatchState(r.Context(), userID, id, req.State); err != nil {
		writeRadarError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) status(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	last, err := h.svc.LastSweep(r.Context(), userID)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, RadarStatus{
		LastSweepAt: last, MaxUserFeeds: h.svc.MaxUserFeeds(),
	})
}

func (h *HTTP) listFeedsScoped(w http.ResponseWriter, r *http.Request, scope FeedScope) {
	q := r.URL.Query()
	params := ListFeedsParams{Scope: scope}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil {
		params.Limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil {
		params.Offset = o
	}

	userID := coreauth.UserID(r.Context())
	result, err := h.svc.ListFeeds(r.Context(), userID, params)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

// UnsubscribeHandler returns the http.HandlerFunc for
// DELETE /radar/subscriptions/{feedId}.
func (h *HTTP) UnsubscribeHandler() http.HandlerFunc {
	return h.unsubscribe
}

func (h *HTTP) unsubscribe(w http.ResponseWriter, r *http.Request) {
	feedID, err := strconv.ParseInt(chi.URLParam(r, "feedId"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
		return
	}

	userID := coreauth.UserID(r.Context())

	if err := h.svc.Unsubscribe(r.Context(), userID, feedID); err != nil {
		writeRadarError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateUserFeedHandler returns the http.HandlerFunc for PATCH /radar/feeds/{id}.
func (h *HTTP) UpdateUserFeedHandler() http.HandlerFunc {
	return h.patchFeed(func(r *http.Request, id int64, req UpdateFeedRequest) (*Feed, error) {
		return h.svc.UpdateUserFeed(r.Context(), coreauth.UserID(r.Context()), id, req)
	})
}

// UpdateGlobalFeedHandler returns the http.HandlerFunc for
// PATCH /admin/radar/feeds/{id} (admin).
func (h *HTTP) UpdateGlobalFeedHandler() http.HandlerFunc {
	return h.patchFeed(func(r *http.Request, id int64, req UpdateFeedRequest) (*Feed, error) {
		return h.svc.UpdateGlobalFeed(r.Context(), id, req)
	})
}

// patchFeed shares the id parsing and body decoding; the scope comes from the
// closure the route was mounted with.
func (h *HTTP) patchFeed(apply func(*http.Request, int64, UpdateFeedRequest) (*Feed, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
			return
		}

		var req UpdateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
			return
		}

		feed, err := apply(r, feedID, req)
		if err != nil {
			writeRadarError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, feed)
	}
}

// DeleteUserFeedHandler returns the http.HandlerFunc for DELETE /radar/feeds/{id}.
func (h *HTTP) DeleteUserFeedHandler() http.HandlerFunc {
	return h.removeFeed(func(r *http.Request, id int64) error {
		return h.svc.DeleteUserFeed(r.Context(), coreauth.UserID(r.Context()), id)
	})
}

// DeleteGlobalFeedHandler returns the http.HandlerFunc for
// DELETE /admin/radar/feeds/{id} (admin).
func (h *HTTP) DeleteGlobalFeedHandler() http.HandlerFunc {
	return h.removeFeed(func(r *http.Request, id int64) error {
		return h.svc.DeleteGlobalFeed(r.Context(), id)
	})
}

func (h *HTTP) removeFeed(apply func(*http.Request, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
			return
		}

		if err := apply(r, feedID); err != nil {
			writeRadarError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
