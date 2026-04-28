package embeddings

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// FakeEmbedder is a deterministic stand-in for TEI used in unit and
// integration tests. Embed returns an L2-normalized vector of length Dim
// derived from SHA-256(text). Same text → same vector; different text →
// different vector with low expected cosine similarity.
type FakeEmbedder struct {
	Dim int
}

func (f *FakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	dim := f.Dim
	if dim <= 0 {
		dim = 1024
	}

	out := make([]float32, dim)
	seed := sha256.Sum256([]byte(text))

	for i := 0; i < dim; i++ {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(i))
		h := sha256.New()
		h.Write(seed[:])
		h.Write(buf[:])
		sum := h.Sum(nil)
		// Turn 4 bytes into a float in roughly [-1, 1].
		u := binary.BigEndian.Uint32(sum[:4])
		f32 := (float32(u)/float32(math.MaxUint32))*2 - 1
		out[i] = f32
	}

	// L2-normalize.
	var norm float64
	for _, x := range out {
		norm += float64(x) * float64(x)
	}

	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range out {
			out[i] = float32(float64(out[i]) / norm)
		}
	}

	return out, nil
}
