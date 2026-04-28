// Package embeddings provides a text-to-vector client interface with a TEI
// (HuggingFace Text Embeddings Inference) implementation and a deterministic
// fake for tests.
package embeddings

import "context"

type Client interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
