package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderScores_printsSimIDAndTitleInOrder(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{
		{ID: 18, Title: new("Wolfenstein 3D for Gameboy"), Similarity: 0.7497},
		{ID: 452, Title: new("WebAuthn и Passkeys"), Similarity: 0.5933},
	}, nil)

	out := buf.String()
	require.Contains(t, out, "0.7497")
	require.Contains(t, out, "18")
	require.Less(t, strings.Index(out, "Wolfenstein"), strings.Index(out, "WebAuthn"))
}

func TestRenderScores_untitledFinding(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{{ID: 3, Title: nil, Similarity: 0.5}}, nil)
	require.Contains(t, buf.String(), "(no title)")
}

func TestRenderScores_truncatesLongTitles(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{
		{ID: 1, Title: new(strings.Repeat("я", 200)), Similarity: 0.5},
	}, nil)

	out := buf.String()
	require.Contains(t, out, "…")
	require.NotContains(t, out, strings.Repeat("я", titleWidth+1))
}

func TestRenderScores_drawsCutoffBetweenRows(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{
		{ID: 1, Title: new("above"), Similarity: 0.60},
		{ID: 2, Title: new("below"), Similarity: 0.50},
	}, new(float32(0.55)))

	out := buf.String()
	cut := strings.Index(out, "threshold 0.5500")
	require.NotEqual(t, -1, cut)
	require.Less(t, strings.Index(out, "above"), cut)
	require.Less(t, cut, strings.Index(out, "below"))
}

func TestRenderScores_cutoffAboveEverything(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{
		{ID: 1, Title: new("below"), Similarity: 0.40},
	}, new(float32(0.55)))

	out := buf.String()
	require.Less(t, strings.Index(out, "threshold 0.5500"), strings.Index(out, "below"))
}

func TestRenderScores_noCutoffWhenEverythingMatches(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, []scoredFinding{
		{ID: 1, Title: new("above"), Similarity: 0.80},
	}, new(float32(0.55)))

	require.NotContains(t, buf.String(), "threshold")
}

func TestRenderScores_empty(t *testing.T) {
	var buf bytes.Buffer
	renderScores(&buf, nil, nil)
	require.Contains(t, buf.String(), "no findings with embeddings")
}

func TestRun_rejectsBadArgsBeforeTouchingTheDatabase(t *testing.T) {
	t.Setenv("LINKTHECA_DB_DSN", "postgres://nobody@127.0.0.1:1/none")

	err := run(context.Background(), []string{"-q", "text", "-limit", "0"}, io.Discard)
	require.ErrorContains(t, err, "limit")
}

func TestParseArgs_helpRequested(t *testing.T) {
	_, err := parseArgs([]string{"-h"})
	require.ErrorIs(t, err, flag.ErrHelp)
}

func TestRun_helpPrintsUsageAndSucceeds(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, run(context.Background(), []string{"-h"}, &buf))

	out := buf.String()
	require.Contains(t, out, "radar-sim")
	require.Contains(t, out, "-topic")
	require.Contains(t, out, "-subscribed")
}

func TestRun_requiresDSN(t *testing.T) {
	t.Setenv("LINKTHECA_DB_DSN", "")

	err := run(context.Background(), []string{"-topics"}, io.Discard)
	require.ErrorContains(t, err, "LINKTHECA_DB_DSN")
}

func TestParseArgs_query(t *testing.T) {
	opts, err := parseArgs([]string{"-q", "webauthn passkeys"})
	require.NoError(t, err)
	require.Equal(t, modeQuery, opts.mode)
	require.Equal(t, "webauthn passkeys", opts.query)
	require.Equal(t, 20, opts.limit)
	require.Nil(t, opts.threshold)
}

func TestParseArgs_topic(t *testing.T) {
	opts, err := parseArgs([]string{"-topic", "7", "-limit", "5"})
	require.NoError(t, err)
	require.Equal(t, modeTopic, opts.mode)
	require.Equal(t, int64(7), opts.topicID)
	require.Equal(t, 5, opts.limit)
}

func TestParseArgs_listTopics(t *testing.T) {
	opts, err := parseArgs([]string{"-topics"})
	require.NoError(t, err)
	require.Equal(t, modeListTopics, opts.mode)
}

func TestParseArgs_thresholdOverride(t *testing.T) {
	opts, err := parseArgs([]string{"-topic", "1", "-threshold", "0.62"})
	require.NoError(t, err)
	require.NotNil(t, opts.threshold)
	require.InDelta(t, 0.62, *opts.threshold, 0.0001)
}

func TestParseArgs_subscribedOnly(t *testing.T) {
	opts, err := parseArgs([]string{"-topic", "1", "-subscribed"})
	require.NoError(t, err)
	require.True(t, opts.subscribedOnly)
}

func TestParseArgs_subscribedNeedsTopic(t *testing.T) {
	_, err := parseArgs([]string{"-q", "text", "-subscribed"})
	require.ErrorContains(t, err, "-subscribed")
}

func TestRenderTopics_showsThresholdAndEmbeddingState(t *testing.T) {
	var buf bytes.Buffer
	renderTopics(&buf, []topicRow{
		{ID: 3, OwnerEmail: "a@example.com", Name: "AI", MatchThreshold: 0.55, IsActive: true, HasEmbedding: true},
		{ID: 4, OwnerEmail: "a@example.com", Name: "Bare", MatchThreshold: 0.70, IsActive: false, HasEmbedding: false},
	})

	out := buf.String()
	require.Contains(t, out, "0.5500")
	require.Contains(t, out, "a@example.com")
	require.Contains(t, out, "AI")
	require.Contains(t, out, "no embedding")
	require.Contains(t, out, "inactive")
}

func TestRenderTopics_empty(t *testing.T) {
	var buf bytes.Buffer
	renderTopics(&buf, nil)
	require.Contains(t, buf.String(), "no topics")
}

func TestParseArgs_noModeIsError(t *testing.T) {
	_, err := parseArgs(nil)
	require.ErrorContains(t, err, "-q")
}

func TestParseArgs_conflictingModesIsError(t *testing.T) {
	_, err := parseArgs([]string{"-q", "text", "-topic", "3"})
	require.ErrorContains(t, err, "exactly one")
}

func TestParseArgs_nonPositiveLimitIsError(t *testing.T) {
	_, err := parseArgs([]string{"-q", "text", "-limit", "0"})
	require.ErrorContains(t, err, "limit")
}

func TestParseArgs_thresholdOutOfRangeIsError(t *testing.T) {
	_, err := parseArgs([]string{"-q", "text", "-threshold", "1.5"})
	require.ErrorContains(t, err, "threshold")
}
