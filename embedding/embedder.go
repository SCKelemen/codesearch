package embedding

import "context"

// Embedder generates vector embeddings for string inputs.
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	Dimensions() int
	Model() string
}
