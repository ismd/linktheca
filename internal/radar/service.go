package radar

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/pgvector/pgvector-go"
)

// defaultMatchThreshold is the match score (see Reference) a finding needs to
// become a match. Picked leave-one-topic-out on 17 labeled topics, where one
// threshold held for two-word topics and one-sentence descriptions alike, see
// docs/superpowers/plans/2026-11-26-radar-hub-corrected-matching.md. Per-topic
// match_threshold lets users tighten or loosen a topic.
const defaultMatchThreshold = .18

// previewLimit leaves room below the cutoff, so a mark that raises it shows
// what drops out.
const previewLimit = 10

// maxTopicFeedback caps the marks one request may carry.
const maxTopicFeedback = 200

// streamMeanTTL is how long the stream's mean match vector is reused. It
// moves with the mix of feeds, not with every finding.
const streamMeanTTL = 10 * time.Minute

// TopicEmbedText is the text a topic is embedded from. Only the description
// takes part: the name is a display label, so renaming a topic never changes
// what it matches.
func TopicEmbedText(description string) string {
	return description
}

const maxDerivedNameRunes = 60

// nameClauseBreak ends the first clause of a description: sentence-final
// punctuation followed by a space (so "Node.js" and "v1.2" survive), a colon
// or semicolon followed by a space (so "https://" survives), or a spaced dash.
var nameClauseBreak = regexp.MustCompile(`[.!?:;](\s|$)|\s[—–-]\s`)

// deriveTopicName builds a display label from the start of a description: the
// first line, up to its first clause break, cut at a word boundary when it is
// still longer than maxDerivedNameRunes. The caller validates the description,
// so it is never blank here.
func deriveTopicName(description string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(description), "\n")

	if loc := nameClauseBreak.FindStringIndex(name); loc != nil && loc[0] > 0 {
		name = name[:loc[0]]
	}
	name = strings.TrimSpace(name)

	runes := []rune(name)
	if len(runes) <= maxDerivedNameRunes {
		return name
	}

	cut := string(runes[:maxDerivedNameRunes])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,-–") + "…"
}

func validateTopicDescription(desc string) error {
	if len(desc) < 10 || len(desc) > 2000 {
		return fmt.Errorf("%w: description must be 10..2000 chars", ErrInvalidInput)
	}
	return nil
}

type StoreAPI interface {
	CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error)
	UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector, m TopicModel) error
	AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error)
	Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error)

	// Read-API extensions:
	ListTopicsWithStats(ctx context.Context, userID int64) ([]TopicWithStats, error)
	GetTopicWithStats(ctx context.Context, userID, topicID int64) (*TopicWithStats, error)
	UpdateTopic(ctx context.Context, userID, topicID int64, p UpdateTopicParams) (*Topic, error)
	DeleteTopic(ctx context.Context, userID, topicID int64) error
	ListMatches(ctx context.Context, userID int64, p ListMatchesParams) ([]MatchView, int, error)
	GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error)
	GetFinding(ctx context.Context, userID, findingID int64) (*MatchFinding, error)
	UpdateMatchState(ctx context.Context, userID, matchID int64, state string) error
	LastSweepAt(ctx context.Context, userID int64) (*time.Time, error)
	ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) ([]FeedListItem, int, error)
	GetGlobalFeedByURL(ctx context.Context, url string) (*Feed, error)
	CountUserFeeds(ctx context.Context, userID int64) (int, error)
	PreviewFindings(ctx context.Context, userID int64, m TopicModel, exclude []int64, limit int) ([]PreviewMatch, error)
	Unsubscribe(ctx context.Context, userID, feedID int64) error
	UpdateFeed(ctx context.Context, feedID int64, owner *int64, p UpdateFeedParams) (*Feed, error)
	DeleteFeed(ctx context.Context, feedID int64, owner *int64) error
	SeedSubscriptions(ctx context.Context, userID int64) (int, error)

	// Learning from the user's marks:
	GetTopicLearnInput(ctx context.Context, topicID int64) (*TopicLearnInput, error)
	ListTopicExamples(ctx context.Context, topicID int64) ([]Example, error)
	FindingExamples(ctx context.Context, userID int64, feedback []TopicFeedback) ([]Example, error)
	SetTopicFeedback(ctx context.Context, userID, topicID, findingID int64, verdict string) error
	ReplaceTopicFeedback(ctx context.Context, userID, topicID int64, feedback []TopicFeedback) error
	DeleteTopicFeedback(ctx context.Context, userID, topicID, findingID int64) error
	ListTopicFeedback(ctx context.Context, userID, topicID int64) ([]TopicFeedbackView, error)
	StreamMean(ctx context.Context, sample int) ([]float32, error)
}

// defaultMaxUserFeeds caps personal feeds per account. An open write endpoint
// with no ceiling is a DoS vector for the crawler and for TEI.
const defaultMaxUserFeeds = 20

type Service struct {
	store        StoreAPI
	embedder     embeddings.Client
	reference    *Reference
	maxUserFeeds int

	meanMu sync.Mutex
	mean   []float32
	meanAt time.Time
}

// ServiceOption tunes optional Service behaviour.
type ServiceOption func(*Service)

// WithReference shares the reference centroid with other users of it, so it is
// embedded once per process. Without it the service embeds its own.
func WithReference(r *Reference) ServiceOption {
	return func(s *Service) {
		if r != nil {
			s.reference = r
		}
	}
}

// WithMaxUserFeeds caps how many personal feeds one account may own. Values
// below 1 are ignored so a missing config cannot lock everyone out.
func WithMaxUserFeeds(n int) ServiceOption {
	return func(s *Service) {
		if n > 0 {
			s.maxUserFeeds = n
		}
	}
}

func NewService(store StoreAPI, embedder embeddings.Client, opts ...ServiceOption) *Service {
	s := &Service{store: store, embedder: embedder, maxUserFeeds: defaultMaxUserFeeds}
	for _, opt := range opts {
		opt(s)
	}

	if s.reference == nil {
		s.reference = NewReference(embedder)
	}

	return s
}

// embedDescription embeds a topic description, wrapping a failure as
// ErrEmbedderUnavailable.
func (s *Service) embedDescription(ctx context.Context, description string) ([]float32, error) {
	q, err := s.embedder.Embed(ctx, TopicEmbedText(description))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedderUnavailable, err)
	}

	return q, nil
}

// learn builds a topic's model from its description embedding q, its
// threshold and the findings marked for it (see LearnTopic).
func (s *Service) learn(ctx context.Context, q []float32, threshold float32, examples []Example) (TopicModel, error) {
	centroid, err := s.reference.Centroid(ctx)
	if err != nil {
		return TopicModel{}, fmt.Errorf("%w: %v", ErrEmbedderUnavailable, err)
	}

	var mean []float32
	for _, e := range examples {
		if e.Relevant {
			if mean, err = s.streamMean(ctx); err != nil {
				return TopicModel{}, err
			}
			break
		}
	}

	return LearnTopic(q, centroid, mean, threshold, examples)
}

// streamMean returns the mean match vector of recent findings, refreshed
// every streamMeanTTL. Nil while there are no findings.
func (s *Service) streamMean(ctx context.Context) ([]float32, error) {
	s.meanMu.Lock()
	defer s.meanMu.Unlock()

	if s.mean != nil && time.Since(s.meanAt) < streamMeanTTL {
		return s.mean, nil
	}

	mean, err := s.store.StreamMean(ctx, streamMeanSample)
	if err != nil {
		return nil, err
	}

	if mean != nil {
		s.mean, s.meanAt = mean, time.Now()
	}

	return mean, nil
}

// relearnTopic rebuilds a saved topic's model from its description embedding
// and marks and stores both. q is the new embedding when the description
// changed, nil to reuse the stored one. A topic never embedded has nothing to
// learn yet: the nil model says so.
func (s *Service) relearnTopic(ctx context.Context, topicID int64, q []float32) (*TopicModel, error) {
	in, err := s.store.GetTopicLearnInput(ctx, topicID)
	if err != nil {
		return nil, err
	}

	if q == nil {
		if in.Embedding == nil {
			return nil, nil
		}
		q = in.Embedding.Slice()
	}

	examples, err := s.store.ListTopicExamples(ctx, topicID)
	if err != nil {
		return nil, err
	}

	model, err := s.learn(ctx, q, in.MatchThreshold, examples)
	if err != nil {
		return nil, err
	}

	if err := s.store.UpdateTopicEmbedding(ctx, topicID, pgvector.NewVector(q), model); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("save topic model: %w", err)
	}

	return &model, nil
}

// ReembedTopic embeds a topic's description again and relearns it from its
// marks. For cmd/reembed-topics, after the way topics are embedded changed.
func (s *Service) ReembedTopic(ctx context.Context, topicID int64) error {
	in, err := s.store.GetTopicLearnInput(ctx, topicID)
	if err != nil {
		return err
	}

	q, err := s.embedDescription(ctx, in.Description)
	if err != nil {
		return err
	}

	_, err = s.relearnTopic(ctx, topicID, q)
	return err
}

// validateFeedback checks marks sent by a client and drops repeats of a
// finding, keeping the last verdict given for it.
func validateFeedback(feedback []TopicFeedback) ([]TopicFeedback, error) {
	if len(feedback) > maxTopicFeedback {
		return nil, fmt.Errorf("%w: at most %d marks", ErrInvalidInput, maxTopicFeedback)
	}

	last := make(map[int64]int, len(feedback))
	for i, f := range feedback {
		if f.FindingID <= 0 {
			return nil, fmt.Errorf("%w: finding_id must be positive", ErrInvalidInput)
		}
		if !validVerdict(f.Verdict) {
			return nil, fmt.Errorf("%w: verdict must be relevant|irrelevant", ErrInvalidInput)
		}
		last[f.FindingID] = i
	}

	out := make([]TopicFeedback, 0, len(last))
	for i, f := range feedback {
		if last[f.FindingID] == i {
			out = append(out, f)
		}
	}

	return out, nil
}

// MaxUserFeeds exposes the personal-feed quota so the UI can show "n / max".
func (s *Service) MaxUserFeeds() int { return s.maxUserFeeds }

// CreateTopic validates the request, persists the topic, and synchronously
// computes its embedding. If the embedder is unavailable the topic stays in
// the database without an embedding and is silently skipped by MatchFindingJob.
// An empty name is derived from the start of the description.
func (s *Service) CreateTopic(ctx context.Context, userID int64, req CreateTopicRequest) (*Topic, error) {
	name := strings.TrimSpace(req.Name)
	desc := strings.TrimSpace(req.Description)

	if len(name) > 200 {
		return nil, fmt.Errorf("%w: name must be at most 200 chars", ErrInvalidInput)
	}

	if err := validateTopicDescription(desc); err != nil {
		return nil, err
	}

	if name == "" {
		name = deriveTopicName(desc)
	}

	threshold := float32(defaultMatchThreshold)
	if req.MatchThreshold != nil {
		threshold = *req.MatchThreshold
		if threshold < 0 || threshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	feedback, err := validateFeedback(req.Feedback)
	if err != nil {
		return nil, err
	}

	topic, err := s.store.CreateTopic(ctx, CreateTopicParams{
		UserID: userID, Name: name, Description: desc, MatchThreshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}

	if len(feedback) > 0 {
		if err := s.store.ReplaceTopicFeedback(ctx, userID, topic.ID, feedback); err != nil {
			return nil, err
		}
	}

	q, err := s.embedDescription(ctx, desc)
	if err != nil {
		return nil, err
	}

	model, err := s.relearnTopic(ctx, topic.ID, q)
	if err != nil {
		return nil, err
	}

	topic.HasEmbedding = true
	topic.MatchCutoff = model.Cutoff
	return topic, nil
}

const (
	defaultFetchIntervalSeconds = 3600
	minFetchIntervalSeconds     = 300
	maxFetchIntervalSeconds     = 86400
)

// validateAddFeed normalises and checks the shared part of both add paths.
func validateAddFeed(req AddFeedRequest) (url, kind string, interval int, err error) {
	url = strings.TrimSpace(req.URL)

	if url == "" || len(url) > 2000 {
		return "", "", 0, fmt.Errorf("%w: url must be 1..2000 chars", ErrInvalidInput)
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", "", 0, fmt.Errorf("%w: url must be http(s)", ErrInvalidInput)
	}

	kind = "rss"
	if req.Kind != nil {
		kind = *req.Kind
		if kind != "rss" && kind != "atom" {
			return "", "", 0, fmt.Errorf("%w: kind must be rss|atom", ErrInvalidInput)
		}
	}

	interval = defaultFetchIntervalSeconds
	if req.FetchIntervalSeconds != nil {
		interval = *req.FetchIntervalSeconds
		if err := validateFetchInterval(interval); err != nil {
			return "", "", 0, err
		}
	}

	return url, kind, interval, nil
}

// AddGlobalFeed creates a catalog feed everyone may subscribe to. Admin scope;
// the /admin/radar route enforces it. Existing users are not auto-subscribed.
func (s *Service) AddGlobalFeed(ctx context.Context, req AddFeedRequest) (*Feed, error) {
	url, kind, interval, err := validateAddFeed(req)
	if err != nil {
		return nil, err
	}

	return s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval,
	})
}

// AddUserFeed creates a personal feed for the caller and subscribes them to it.
// If the URL is already in the shared catalog no row is created: the caller is
// simply subscribed to the catalog feed, so a popular feed is not duplicated
// once per account.
func (s *Service) AddUserFeed(ctx context.Context, userID int64, req AddFeedRequest) (*AddFeedResult, error) {
	url, kind, interval, err := validateAddFeed(req)
	if err != nil {
		return nil, err
	}

	switch existing, err := s.store.GetGlobalFeedByURL(ctx, url); {
	case err == nil:
		if _, err := s.store.Subscribe(ctx, userID, existing.ID); err != nil {
			return nil, err
		}

		return &AddFeedResult{Feed: existing, Created: false}, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}

	// Best-effort ceiling: two concurrent adds can land one feed over the
	// limit. A transaction here would buy nothing worth its cost.
	n, err := s.store.CountUserFeeds(ctx, userID)
	if err != nil {
		return nil, err
	}

	if n >= s.maxUserFeeds {
		return nil, fmt.Errorf("%w: at most %d personal feeds", ErrQuotaExceeded, s.maxUserFeeds)
	}

	feed, err := s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval, OwnerUserID: &userID,
	})
	if err != nil {
		return nil, err
	}

	if _, err := s.store.Subscribe(ctx, userID, feed.ID); err != nil {
		return nil, err
	}

	return &AddFeedResult{Feed: feed, Created: true}, nil
}

func (s *Service) Subscribe(ctx context.Context, userID int64, req SubscribeRequest) (*Subscription, error) {
	if req.FeedID <= 0 {
		return nil, fmt.Errorf("%w: feed_id must be positive", ErrInvalidInput)
	}

	return s.store.Subscribe(ctx, userID, req.FeedID)
}

// ListTopics returns all topics of a user with aggregate match stats.
func (s *Service) ListTopics(ctx context.Context, userID int64) ([]TopicWithStats, error) {
	return s.store.ListTopicsWithStats(ctx, userID)
}

// GetTopic returns a single topic with aggregate match stats, or ErrNotFound.
func (s *Service) GetTopic(ctx context.Context, userID, topicID int64) (*TopicWithStats, error) {
	return s.store.GetTopicWithStats(ctx, userID, topicID)
}

// DeleteTopic removes a topic and CASCADEs its matches.
func (s *Service) DeleteTopic(ctx context.Context, userID, topicID int64) error {
	return s.store.DeleteTopic(ctx, userID, topicID)
}

// UpdateTopic validates the patch, persists changed fields and marks, and
// relearns the topic when its description, threshold or marks changed — a
// new description is embedded first. Mirrors CreateTopic: embedder failure
// leaves the topic's fields updated and its model stale, and returns
// ErrEmbedderUnavailable. The caller can retry with the same payload.
func (s *Service) UpdateTopic(ctx context.Context, userID, topicID int64, req UpdateTopicRequest) (*Topic, error) {
	p := UpdateTopicParams{
		Name: req.Name, Description: req.Description,
		MatchThreshold: req.MatchThreshold, IsActive: req.IsActive,
	}
	fieldsChanged := p.Name != nil || p.Description != nil || p.MatchThreshold != nil || p.IsActive != nil

	if !fieldsChanged && req.Feedback == nil {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	var feedback []TopicFeedback
	if req.Feedback != nil {
		var err error
		if feedback, err = validateFeedback(*req.Feedback); err != nil {
			return nil, err
		}
	}

	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 200 {
			return nil, fmt.Errorf("%w: name must be 1..200 chars", ErrInvalidInput)
		}
		p.Name = &n
	}
	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		if err := validateTopicDescription(d); err != nil {
			return nil, err
		}
		p.Description = &d
	}
	if p.MatchThreshold != nil {
		if *p.MatchThreshold < 0 || *p.MatchThreshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	topic, err := s.updateTopicFields(ctx, userID, topicID, p, fieldsChanged)
	if err != nil {
		return nil, err
	}

	if req.Feedback != nil {
		if err := s.store.ReplaceTopicFeedback(ctx, userID, topicID, feedback); err != nil {
			return nil, err
		}
	}

	if p.Description == nil && p.MatchThreshold == nil && req.Feedback == nil {
		return topic, nil
	}

	var q []float32
	if p.Description != nil {
		if q, err = s.embedDescription(ctx, topic.Description); err != nil {
			return nil, err
		}
	}

	model, err := s.relearnTopic(ctx, topic.ID, q)
	if err != nil {
		return nil, err
	}

	if model != nil {
		topic.HasEmbedding = true
		topic.MatchCutoff = model.Cutoff
	}

	return topic, nil
}

// updateTopicFields saves the topic's own fields, or only checks the topic is
// the user's when the patch carries marks alone.
func (s *Service) updateTopicFields(ctx context.Context, userID, topicID int64, p UpdateTopicParams, changed bool) (*Topic, error) {
	if changed {
		return s.store.UpdateTopic(ctx, userID, topicID, p)
	}

	t, err := s.store.GetTopicWithStats(ctx, userID, topicID)
	if err != nil {
		return nil, err
	}

	return &t.Topic, nil
}

// SetTopicFeedback records the user's verdict on a finding for one of their
// topics and relearns the topic. A finding marked irrelevant leaves the
// topic's matches until the mark is taken back.
func (s *Service) SetTopicFeedback(ctx context.Context, userID, topicID, findingID int64, verdict string) error {
	if findingID <= 0 {
		return fmt.Errorf("%w: finding id must be positive", ErrInvalidInput)
	}
	if !validVerdict(verdict) {
		return fmt.Errorf("%w: verdict must be relevant|irrelevant", ErrInvalidInput)
	}

	if err := s.store.SetTopicFeedback(ctx, userID, topicID, findingID, verdict); err != nil {
		return err
	}

	_, err := s.relearnTopic(ctx, topicID, nil)
	return err
}

// DeleteTopicFeedback takes back the user's verdict on a finding for one of
// their topics and relearns the topic.
func (s *Service) DeleteTopicFeedback(ctx context.Context, userID, topicID, findingID int64) error {
	if err := s.store.DeleteTopicFeedback(ctx, userID, topicID, findingID); err != nil {
		return err
	}

	_, err := s.relearnTopic(ctx, topicID, nil)
	return err
}

// ListTopicFeedback returns the marks on one of the user's topics.
func (s *Service) ListTopicFeedback(ctx context.Context, userID, topicID int64) ([]TopicFeedbackView, error) {
	if _, err := s.store.GetTopicWithStats(ctx, userID, topicID); err != nil {
		return nil, err
	}

	return s.store.ListTopicFeedback(ctx, userID, topicID)
}

// PreviewTopic scores the subscribed findings against a draft description and
// the marks given so far, learned exactly the way CreateTopic would learn
// them. Findings marked irrelevant are left out.
func (s *Service) PreviewTopic(ctx context.Context, userID int64, req PreviewTopicRequest) (*TopicPreview, error) {
	desc := strings.TrimSpace(req.Description)

	if err := validateTopicDescription(desc); err != nil {
		return nil, err
	}

	feedback, err := validateFeedback(req.Feedback)
	if err != nil {
		return nil, err
	}

	q, err := s.embedDescription(ctx, desc)
	if err != nil {
		return nil, err
	}

	examples, err := s.store.FindingExamples(ctx, userID, feedback)
	if err != nil {
		return nil, err
	}

	model, err := s.learn(ctx, q, defaultMatchThreshold, examples)
	if err != nil {
		return nil, err
	}

	exclude := []int64{}
	for _, f := range feedback {
		if f.Verdict == VerdictIrrelevant {
			exclude = append(exclude, f.FindingID)
		}
	}

	items, err := s.store.PreviewFindings(ctx, userID, model, exclude, previewLimit)
	if err != nil {
		return nil, fmt.Errorf("preview findings: %w", err)
	}

	return &TopicPreview{Items: items, Threshold: model.Cutoff}, nil
}

// ListMatches returns paginated matches for a user, optionally filtered by
// topic and/or state. Clamps Limit to [1,100] (default 50) and Offset to >=0.
func (s *Service) ListMatches(ctx context.Context, p ListMatchesParams) (*MatchList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	items, total, err := s.store.ListMatches(ctx, p.UserID, p)
	if err != nil {
		return nil, err
	}
	return &MatchList{Items: items, Total: total}, nil
}

// SetMatchState updates a match's state. Valid states: "new", "seen".
func (s *Service) SetMatchState(ctx context.Context, userID, matchID int64, state string) error {
	if state != "new" && state != "seen" {
		return fmt.Errorf("%w: state must be new|seen", ErrInvalidInput)
	}
	return s.store.UpdateMatchState(ctx, userID, matchID, state)
}

// GetMatch returns a single denormalized match owned by the user.
// The article HTML is sanitized here: the frontend renders it as is.
func (s *Service) GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error) {
	mv, err := s.store.GetMatch(ctx, userID, matchID)
	if err != nil {
		return nil, err
	}

	sanitizeFindingContent(&mv.Finding)
	return mv, nil
}

// GetFinding returns a finding from one of the user's subscribed feeds, with
// its article, so a topic preview row can be read before it becomes a match.
func (s *Service) GetFinding(ctx context.Context, userID, findingID int64) (*MatchFinding, error) {
	f, err := s.store.GetFinding(ctx, userID, findingID)
	if err != nil {
		return nil, err
	}

	sanitizeFindingContent(f)
	return f, nil
}

// sanitizeFindingContent cleans the stored article HTML, which the frontend
// renders as is, and drops it when nothing survives.
func sanitizeFindingContent(f *MatchFinding) {
	c := f.Content
	if c == nil || c.HTML == nil {
		return
	}

	clean := content.SanitizeHTML(*c.HTML)
	c.HTML = &clean
	if clean == "" {
		c.HTML = nil
	}
}

// LastSweep returns the latest fetch timestamp across the user's active
// subscribed feeds, or nil if there are no subscriptions.
func (s *Service) LastSweep(ctx context.Context, userID int64) (*time.Time, error) {
	return s.store.LastSweepAt(ctx, userID)
}

// ListFeeds returns the instance feed catalog with the caller's subscription
// state on every row.
func (s *Service) ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) (*FeedList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}

	if p.Offset < 0 {
		p.Offset = 0
	}

	if p.Scope == "" {
		p.Scope = FeedScopeVisible
	}

	items, total, err := s.store.ListFeeds(ctx, userID, p)
	if err != nil {
		return nil, err
	}

	return &FeedList{Items: items, Total: total}, nil
}

func (s *Service) Unsubscribe(ctx context.Context, userID, feedID int64) error {
	if feedID <= 0 {
		return fmt.Errorf("%w: feed_id must be positive", ErrInvalidInput)
	}

	return s.store.Unsubscribe(ctx, userID, feedID)
}

func validateFetchInterval(seconds int) error {
	if seconds < minFetchIntervalSeconds || seconds > maxFetchIntervalSeconds {
		return fmt.Errorf("%w: fetch_interval_seconds must be %d..%d",
			ErrInvalidInput, minFetchIntervalSeconds, maxFetchIntervalSeconds)
	}
	return nil
}

// updateFeed patches one feed inside a single ownership scope. Callers pick the
// scope by choosing UpdateUserFeed or UpdateGlobalFeed, so a handler cannot
// reach outside the namespace it is mounted on.
func (s *Service) updateFeed(ctx context.Context, owner *int64, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	if feedID <= 0 {
		return nil, fmt.Errorf("%w: feed id must be positive", ErrInvalidInput)
	}
	if req.Title == nil && req.FetchIntervalSeconds == nil && req.IsActive == nil {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}
	if req.FetchIntervalSeconds != nil {
		if err := validateFetchInterval(*req.FetchIntervalSeconds); err != nil {
			return nil, err
		}
	}
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if len(trimmed) > 500 {
			return nil, fmt.Errorf("%w: title must be at most 500 chars", ErrInvalidInput)
		}
		req.Title = &trimmed
	}

	return s.store.UpdateFeed(ctx, feedID, owner, UpdateFeedParams{
		Title:                req.Title,
		FetchIntervalSeconds: req.FetchIntervalSeconds,
		IsActive:             req.IsActive,
	})
}

// UpdateUserFeed patches one of the caller's own personal feeds.
func (s *Service) UpdateUserFeed(ctx context.Context, userID, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	return s.updateFeed(ctx, &userID, feedID, req)
}

// UpdateGlobalFeed patches a catalog feed. Admin scope.
func (s *Service) UpdateGlobalFeed(ctx context.Context, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	return s.updateFeed(ctx, nil, feedID, req)
}

func (s *Service) deleteFeed(ctx context.Context, owner *int64, feedID int64) error {
	if feedID <= 0 {
		return fmt.Errorf("%w: feed id must be positive", ErrInvalidInput)
	}

	return s.store.DeleteFeed(ctx, feedID, owner)
}

// DeleteUserFeed removes one of the caller's own personal feeds.
func (s *Service) DeleteUserFeed(ctx context.Context, userID, feedID int64) error {
	return s.deleteFeed(ctx, &userID, feedID)
}

// DeleteGlobalFeed removes a catalog feed for everyone. Admin scope.
func (s *Service) DeleteGlobalFeed(ctx context.Context, feedID int64) error {
	return s.deleteFeed(ctx, nil, feedID)
}

// SeedSubscriptions is called from the auth module's OnUserCreated hook.
func (s *Service) SeedSubscriptions(ctx context.Context, userID int64) error {
	_, err := s.store.SeedSubscriptions(ctx, userID)
	return err
}
