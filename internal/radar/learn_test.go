package radar_test

import (
	"math"
	"testing"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

// unit returns the unit vector along the given components.
func unit(xs ...float32) []float32 {
	var n float64
	for _, x := range xs {
		n += float64(x) * float64(x)
	}
	out := make([]float32, len(xs))
	for i, x := range xs {
		out[i] = float32(float64(x) / math.Sqrt(n))
	}
	return out
}

func dotf(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

var (
	learnQ        = unit(1, 0, 0, 0)
	learnCentroid = []float32{0.1, 0.1, 0.1, 0.1}
)

func TestLearnTopic_withoutMarksIsTheDescription(t *testing.T) {
	m, err := radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, nil)
	require.NoError(t, err)

	require.InDeltaSlice(t, []float32{0.9, -0.1, -0.1, -0.1}, m.MatchVec, 1e-6)
	require.Zero(t, m.Offset)
	require.Equal(t, float32(0.18), m.Cutoff)
}

func TestLearnTopic_irrelevantRaisesTheCutoffAboveIt(t *testing.T) {
	noise := []float32{0.5, 0.2, 0.1, 0}
	quiet := []float32{0.1, 0, 0, 0}

	m, err := radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{
		{FindingID: 1, Vec: noise}, {FindingID: 2, Vec: quiet},
	})
	require.NoError(t, err)

	require.InDelta(t, m.Score(noise)+0.005, m.Cutoff, 1e-6, "just above the highest rejected finding")
	require.Less(t, m.Score(noise), m.Cutoff)
	require.Zero(t, m.Offset, "✗ does not move the topic")

	// A rejected finding that scores below the threshold leaves it alone.
	m, err = radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{{Vec: quiet}})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), m.Cutoff)
}

func TestLearnTopic_relevantCapsTheCutoffBelowIt(t *testing.T) {
	// Steam Frame against Steam Deck: the rejected finding scores above a
	// real one the user confirmed, so the cutoff stays below the real one.
	rejected := []float32{0.6, 0.2, 0, 0}
	confirmed := []float32{0.45, 0, 0.1, 0}

	m, err := radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{
		{Vec: rejected}, {Vec: confirmed, Relevant: true},
	})
	require.NoError(t, err)
	require.InDelta(t, m.Score(confirmed)-0.005, m.Cutoff, 1e-6)
	require.GreaterOrEqual(t, m.Score(confirmed), m.Cutoff)

	// Never below the threshold, however low the confirmed finding scores.
	low := []float32{0.02, 0, 0, 0.1}
	m, err = radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{
		{Vec: rejected}, {Vec: low, Relevant: true},
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), m.Cutoff)
}

func TestLearnTopic_relevantMovesTheTopicAndKeepsTheStreamLevel(t *testing.T) {
	example := []float32{0, 0.4, 0, 0}
	stream := []float32{0.2, 0.2, 0.2, 0.2}

	plain, err := radar.LearnTopic(learnQ, learnCentroid, stream, 0.18, nil)
	require.NoError(t, err)
	m, err := radar.LearnTopic(learnQ, learnCentroid, stream, 0.18, []radar.Example{{Vec: example, Relevant: true}})
	require.NoError(t, err)

	// q' = normalize(q + unit(example)) is halfway between the two axes.
	moved := unit(1, 1, 0, 0)
	for i := range moved {
		require.InDelta(t, moved[i]-learnCentroid[i], m.MatchVec[i], 1e-6)
	}

	require.Greater(t, m.Score(example), plain.Score(example), "the confirmed finding scores higher")
	require.InDelta(t, plain.Score(stream), m.Score(stream), 1e-6,
		"the offset keeps an average finding where it was")
	require.InDelta(t, dotf(stream, moved)-dotf(stream, learnQ), m.Offset, 1e-6)
}

func TestLearnTopic_withoutAStreamMeanThereIsNoOffset(t *testing.T) {
	m, err := radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{
		{Vec: []float32{0, 0.4, 0, 0}, Relevant: true},
	})
	require.NoError(t, err)
	require.Zero(t, m.Offset)
}

func TestLearnTopic_rejectsMismatchedDimensions(t *testing.T) {
	_, err := radar.LearnTopic(learnQ, learnCentroid[:3], nil, 0.18, nil)
	require.ErrorContains(t, err, "dimensions")

	_, err = radar.LearnTopic(learnQ, learnCentroid, nil, 0.18, []radar.Example{{Vec: []float32{1, 0}}})
	require.ErrorContains(t, err, "dimensions")
}
