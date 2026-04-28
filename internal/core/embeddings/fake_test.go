package embeddings

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeEmbedder_Deterministic(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	ctx := context.Background()

	v1, err := e.Embed(ctx, "hello world")
	require.NoError(t, err)
	require.Len(t, v1, 1024)

	v2, err := e.Embed(ctx, "hello world")
	require.NoError(t, err)
	require.Equal(t, v1, v2)
}

func TestFakeEmbedder_Differs(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	ctx := context.Background()

	a, _ := e.Embed(ctx, "alpha")
	b, _ := e.Embed(ctx, "bravo")
	require.NotEqual(t, a, b)
}

func TestFakeEmbedder_Normalized(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	v, _ := e.Embed(context.Background(), "anything")

	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	require.InDelta(t, 1.0, math.Sqrt(sum), 1e-5)
}
