package radar_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

func TestReference_CentroidIsTheMeanAndIsCached(t *testing.T) {
	counter := &countingEmbedder{inner: &embeddings.FakeEmbedder{Dim: 8}}
	ref := radar.NewReference(counter)
	ctx := context.Background()

	c, err := ref.Centroid(ctx)
	require.NoError(t, err)
	require.Len(t, c, 8)
	embedded := counter.calls
	require.Greater(t, embedded, 50, "every reference topic is embedded")

	again, err := ref.Centroid(ctx)
	require.NoError(t, err)
	require.Equal(t, c, again)
	require.Equal(t, embedded, counter.calls, "the centroid is embedded once")

	// The mean of unit vectors in random directions is much shorter than one.
	var norm float32
	for _, x := range c {
		norm += x * x
	}
	require.Less(t, norm, float32(0.5))
}

// flakyEmbedder fails its first n calls.
type flakyEmbedder struct {
	inner embeddings.Client
	fails int
}

func (f *flakyEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if f.fails > 0 {
		f.fails--
		return nil, errors.New("tei down")
	}
	return f.inner.Embed(ctx, text)
}

func TestReference_FailureIsNotCached(t *testing.T) {
	ref := radar.NewReference(&flakyEmbedder{inner: &embeddings.FakeEmbedder{Dim: 8}, fails: 1})
	ctx := context.Background()

	_, err := ref.Centroid(ctx)
	require.ErrorContains(t, err, "tei down")

	c, err := ref.Centroid(ctx)
	require.NoError(t, err)
	require.Len(t, c, 8)
}

func TestReference_TopicMatchVectorSubtractsTheCentroid(t *testing.T) {
	emb := &embeddings.FakeEmbedder{Dim: 8}
	ref := radar.NewReference(emb)
	ctx := context.Background()

	c, err := ref.Centroid(ctx)
	require.NoError(t, err)

	q, _ := emb.Embed(ctx, "webauthn passkeys")
	got, err := ref.TopicMatchVector(ctx, q)
	require.NoError(t, err)
	for i := range q {
		require.InDelta(t, q[i]-c[i], got[i], 1e-7)
	}

	_, err = ref.TopicMatchVector(ctx, q[:4])
	require.ErrorContains(t, err, "dimensions")
}

func TestFindingMatchVector_isTheMean(t *testing.T) {
	got := radar.FindingMatchVector([]float32{1, 0, 0.5}, []float32{0, 1, 0.5})
	require.Equal(t, []float32{0.5, 0.5, 0.5}, got)
}
