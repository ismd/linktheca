package radar_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// feedbackFixture is a user with a topic, a subscribed feed and an
// unsubscribed one, and a finding in each.
type feedbackFixture struct {
	user, topic         int64
	visible, invisible  int64
	subscribed, foreign int64
}

func seedFeedbackFixture(t *testing.T, pool *pgxpool.Pool) feedbackFixture {
	t.Helper()

	var f feedbackFixture
	f.user = seedUser(t, pool)
	f.subscribed = seedFeed(t, pool, "https://f.example/"+t.Name()+"/sub", "Subscribed")
	f.foreign = seedFeed(t, pool, "https://f.example/"+t.Name()+"/other", "Other")
	seedSubscription(t, pool, f.user, f.subscribed)
	f.topic = seedTopic(t, pool, f.user, "t", "topic", 0.18, true)
	f.visible = seedFinding(t, pool, f.subscribed, "https://p.example/visible", "Visible")
	f.invisible = seedFinding(t, pool, f.foreign, "https://p.example/invisible", "Invisible")
	seedFindingEmbedding(t, pool, f.visible, 0)
	seedFindingEmbedding(t, pool, f.invisible, 1)

	return f
}

func TestStore_TopicFeedback_setListDelete(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)
	stranger := seedUser(t, pool)

	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictRelevant))
	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictIrrelevant),
		"marking again changes the verdict")

	require.ErrorIs(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.invisible, radar.VerdictRelevant),
		radar.ErrNotFound, "findings outside the user's feeds cannot be marked")
	require.ErrorIs(t, store.SetTopicFeedback(ctx, stranger, f.topic, f.visible, radar.VerdictRelevant),
		radar.ErrNotFound, "nor can someone else's topic")

	items, err := store.ListTopicFeedback(ctx, f.user, f.topic)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, radar.VerdictIrrelevant, items[0].Verdict)
	require.Equal(t, f.visible, items[0].Finding.ID)
	require.Equal(t, "Visible", *items[0].Finding.Title)
	require.Equal(t, "Subscribed", *items[0].Finding.FeedTitle)

	none, err := store.ListTopicFeedback(ctx, stranger, f.topic)
	require.NoError(t, err)
	require.Empty(t, none)

	topic, err := store.GetTopicWithStats(ctx, f.user, f.topic)
	require.NoError(t, err)
	require.Equal(t, 0, topic.Stats.MarkedRelevant)
	require.Equal(t, 1, topic.Stats.MarkedIrrelevant)

	require.ErrorIs(t, store.DeleteTopicFeedback(ctx, stranger, f.topic, f.visible), radar.ErrNotFound)
	require.NoError(t, store.DeleteTopicFeedback(ctx, f.user, f.topic, f.visible))
	require.ErrorIs(t, store.DeleteTopicFeedback(ctx, f.user, f.topic, f.visible), radar.ErrNotFound)
}

func TestStore_ReplaceTopicFeedback_keepsMarksTheUserCannotSee(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)
	another := seedFinding(t, pool, f.subscribed, "https://p.example/another", "Another")

	// A mark made while the user still subscribed to the other feed.
	_, err := pool.Exec(ctx,
		`INSERT INTO radar_topic_feedback (topic_id, finding_id, verdict) VALUES ($1, $2, 'irrelevant')`,
		f.topic, f.invisible)
	require.NoError(t, err)
	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictIrrelevant))

	require.NoError(t, store.ReplaceTopicFeedback(ctx, f.user, f.topic, []radar.TopicFeedback{
		{FindingID: another, Verdict: radar.VerdictRelevant},
		{FindingID: f.invisible, Verdict: radar.VerdictRelevant},
	}))

	require.Equal(t, map[int64]string{another: "relevant", f.invisible: "irrelevant"}, feedbackRows(t, pool, f.topic),
		"the visible mark is replaced, the hidden one is neither dropped nor changed")

	require.NoError(t, store.ReplaceTopicFeedback(ctx, f.user, f.topic, []radar.TopicFeedback{}))
	require.Equal(t, map[int64]string{f.invisible: "irrelevant"}, feedbackRows(t, pool, f.topic))

	// Someone else's topic is left alone.
	stranger := seedUser(t, pool)
	seedSubscription(t, pool, stranger, f.subscribed)
	require.NoError(t, store.ReplaceTopicFeedback(ctx, stranger, f.topic, []radar.TopicFeedback{
		{FindingID: another, Verdict: radar.VerdictRelevant},
	}))
	require.Equal(t, map[int64]string{f.invisible: "irrelevant"}, feedbackRows(t, pool, f.topic))
}

func feedbackRows(t *testing.T, pool *pgxpool.Pool, topicID int64) map[int64]string {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		`SELECT finding_id, verdict FROM radar_topic_feedback WHERE topic_id = $1`, topicID)
	require.NoError(t, err)
	defer rows.Close()

	out := map[int64]string{}
	for rows.Next() {
		var (
			id      int64
			verdict string
		)
		require.NoError(t, rows.Scan(&id, &verdict))
		out[id] = verdict
	}
	require.NoError(t, rows.Err())

	return out
}

// ✗ hides a match without deleting it, so taking the mark back restores it.
func TestStore_IrrelevantMarkHidesTheMatch(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)
	match := seedMatch(t, pool, f.topic, f.visible, "new", 0.4)

	count := func() (int, int, int) {
		t.Helper()
		items, total, err := store.ListMatches(ctx, f.user, radar.ListMatchesParams{Limit: 10})
		require.NoError(t, err)
		topic, err := store.GetTopicWithStats(ctx, f.user, f.topic)
		require.NoError(t, err)
		require.Equal(t, topic.Stats.TotalCount, topic.Stats.NewCount)
		return len(items), total, topic.Stats.TotalCount
	}

	listed, total, stats := count()
	require.Equal(t, []int{1, 1, 1}, []int{listed, total, stats})

	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictIrrelevant))
	listed, total, stats = count()
	require.Equal(t, []int{0, 0, 0}, []int{listed, total, stats})

	// The reader can still open it, to take the mark back.
	hidden, err := store.GetMatch(ctx, f.user, match)
	require.NoError(t, err)
	require.Equal(t, radar.VerdictIrrelevant, *hidden.Verdict)

	require.NoError(t, store.DeleteTopicFeedback(ctx, f.user, f.topic, f.visible))
	listed, total, stats = count()
	require.Equal(t, []int{1, 1, 1}, []int{listed, total, stats})

	// A ✓ shows on the listed match.
	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictRelevant))
	items, _, err := store.ListMatches(ctx, f.user, radar.ListMatchesParams{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, radar.VerdictRelevant, *items[0].Verdict)
}

func TestStore_MatchFindingToTopics_usesTheLearnedModel(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)

	// The finding's match vector is axis 0, so its raw score is 1; with the
	// offset it scores 0.7.
	setModel := func(offset, cutoff float32) {
		t.Helper()
		require.NoError(t, store.UpdateTopicEmbedding(ctx, f.topic, unitVec(1),
			radar.TopicModel{MatchVec: unitVec(1).Slice(), Offset: offset, Cutoff: cutoff}))
	}

	setModel(0.3, 0.75)
	n, err := store.MatchFindingToTopics(ctx, f.visible)
	require.NoError(t, err)
	require.Zero(t, n, "0.7 is below the cutoff")

	setModel(0.3, 0.65)
	n, err = store.MatchFindingToTopics(ctx, f.visible)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	items, _, err := store.ListMatches(ctx, f.user, radar.ListMatchesParams{Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.InDelta(t, 0.7, items[0].Similarity, 0.001, "the stored score has the offset taken off")
}

func TestStore_MatchFindingToTopics_skipsRejectedFindings(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)

	require.NoError(t, store.UpdateTopicEmbedding(ctx, f.topic, unitVec(1),
		radar.TopicModel{MatchVec: unitVec(1).Slice(), Cutoff: 0.18}))
	require.NoError(t, store.SetTopicFeedback(ctx, f.user, f.topic, f.visible, radar.VerdictIrrelevant))

	n, err := store.MatchFindingToTopics(ctx, f.visible)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestStore_PreviewFindings_subtractsTheOffsetAndLeavesOutExcluded(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)
	other := seedFinding(t, pool, f.subscribed, "https://p.example/other", "Other")
	seedFindingEmbedding(t, pool, other, 0)

	items, err := store.PreviewFindings(ctx, f.user,
		radar.TopicModel{MatchVec: unitVec(1).Slice(), Offset: 0.25}, []int64{f.visible}, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, other, items[0].Finding.ID)
	require.InDelta(t, 0.75, items[0].Similarity, 0.001)
}

func TestStore_LearningInputs(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()
	f := seedFeedbackFixture(t, pool)
	unembedded := seedFinding(t, pool, f.subscribed, "https://p.example/raw", "Raw")

	in, err := store.GetTopicLearnInput(ctx, f.topic)
	require.NoError(t, err)
	require.Equal(t, "topic", in.Description)
	require.Equal(t, float32(0.18), in.MatchThreshold)
	require.Nil(t, in.Embedding)

	require.NoError(t, store.UpdateTopicEmbedding(ctx, f.topic, unitVec(1),
		radar.TopicModel{MatchVec: unitVec(1).Slice(), Cutoff: 0.18}))
	in, err = store.GetTopicLearnInput(ctx, f.topic)
	require.NoError(t, err)
	require.NotNil(t, in.Embedding)

	_, err = store.GetTopicLearnInput(ctx, 424242)
	require.ErrorIs(t, err, radar.ErrNotFound)

	// Stored marks teach the topic even once their feed is unsubscribed.
	_, err = pool.Exec(ctx, `
		INSERT INTO radar_topic_feedback (topic_id, finding_id, verdict)
		VALUES ($1, $2, 'relevant'), ($1, $3, 'irrelevant'), ($1, $4, 'relevant')`,
		f.topic, f.visible, f.invisible, unembedded)
	require.NoError(t, err)

	examples, err := store.ListTopicExamples(ctx, f.topic)
	require.NoError(t, err)
	require.Len(t, examples, 2, "a finding without a match vector teaches nothing")
	byID := map[int64]radar.Example{}
	for _, e := range examples {
		byID[e.FindingID] = e
	}
	require.True(t, byID[f.visible].Relevant)
	require.False(t, byID[f.invisible].Relevant)
	require.Equal(t, unitVec(1).Slice(), byID[f.visible].Vec)

	// Marks a client sends for a draft only count on the user's own findings.
	drafts, err := store.FindingExamples(ctx, f.user, []radar.TopicFeedback{
		{FindingID: f.visible, Verdict: radar.VerdictIrrelevant},
		{FindingID: f.invisible, Verdict: radar.VerdictRelevant},
		{FindingID: unembedded, Verdict: radar.VerdictRelevant},
	})
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	require.Equal(t, f.visible, drafts[0].FindingID)
	require.False(t, drafts[0].Relevant)
}

func TestStore_StreamMean(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	mean, err := store.StreamMean(ctx, 10)
	require.NoError(t, err)
	require.Nil(t, mean, "no findings, no stream")

	f := seedFeedbackFixture(t, pool)
	_, err = pool.Exec(ctx, `UPDATE radar_findings SET discovered_at = now() - interval '1 hour' WHERE id = $1`, f.invisible)
	require.NoError(t, err)

	mean, err = store.StreamMean(ctx, 10)
	require.NoError(t, err)
	require.InDelta(t, 0.5, mean[0], 1e-6)
	require.InDelta(t, 0.5, mean[1], 1e-6)

	mean, err = store.StreamMean(ctx, 1)
	require.NoError(t, err)
	require.InDelta(t, 1, mean[0], 1e-6, "only the latest findings count")
	require.InDelta(t, 0, mean[1], 1e-6)
}
