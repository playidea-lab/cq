package llm

import "context"

// Embedder is the interface for generating text embeddings.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	EmbedDimension() int
}

// EmbedRequest holds parameters for an embedding API call.
type EmbedRequest struct {
	Model  string   `json:"model"`
	Inputs []string `json:"input"`
}

// EmbedResponse holds the result of an embedding API call.
type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Usage      TokenUsage  `json:"usage"`
}

// EmbeddingProvider wraps a Gateway to implement the Embedder interface.
// taskType is used for routing (typically "embedding").
type EmbeddingProvider struct {
	gateway   *Gateway
	taskType  string
	dimension int
}

// NewEmbeddingProvider creates an Embedder backed by a Gateway.
func NewEmbeddingProvider(gw *Gateway, dimension int) *EmbeddingProvider {
	return &EmbeddingProvider{
		gateway:   gw,
		taskType:  "embedding",
		dimension: dimension,
	}
}

func (p *EmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.gateway.Embed(ctx, p.taskType, texts)
}

func (p *EmbeddingProvider) EmbedDimension() int {
	return p.dimension
}
