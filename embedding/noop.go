package embedding

import "context"

// NoopEmbedder returns zero vectors for testing.
type NoopEmbedder struct {
	dimensions int
	model      string
}

// NewNoopEmbedder creates a test embedder with the requested dimensions.
func NewNoopEmbedder(dimensions int) NoopEmbedder {
	if dimensions < 0 {
		dimensions = 0
	}
	return NoopEmbedder{
		dimensions: dimensions,
		model:      "noop",
	}
}

// Embed returns one zero vector per input.
func (n NoopEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for i := range inputs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		vectors[i] = make([]float32, n.dimensions)
	}
	return vectors, nil
}

// Dimensions returns the configured vector size.
func (n NoopEmbedder) Dimensions() int {
	return n.dimensions
}

// Model returns the embedder name.
func (n NoopEmbedder) Model() string {
	if n.model == "" {
		return "noop"
	}
	return n.model
}
