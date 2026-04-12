package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LocalEmbedder calls a local HTTP embedding endpoint.
type LocalEmbedder struct {
	url    string
	client *http.Client

	mu         sync.RWMutex
	dimensions int
	model      string
}

// NewLocalEmbedder creates an embedder backed by a local HTTP endpoint.
func NewLocalEmbedder(url string) *LocalEmbedder {
	return &LocalEmbedder{
		url: url,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		model: "local",
	}
}

// Embed posts inputs to the configured endpoint and returns the decoded embeddings.
func (l *LocalEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(localEmbedRequest{Inputs: inputs, Input: inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal local embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create local embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send local embed request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("local embed request failed: %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}

	var decoded localEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode local embed response: %w", err)
	}
	if decoded.Error != "" {
		return nil, fmt.Errorf("local embed request failed: %s", decoded.Error)
	}

	embeddings := decoded.Embeddings
	if len(embeddings) == 0 && len(decoded.Data) > 0 {
		embeddings = make([][]float32, len(decoded.Data))
		for i, item := range decoded.Data {
			embeddings[i] = item.Embedding
		}
	}
	if len(embeddings) != len(inputs) {
		return nil, fmt.Errorf("local embed response returned %d embeddings for %d inputs", len(embeddings), len(inputs))
	}

	dimensions := decoded.Dimensions
	if dimensions == 0 && len(embeddings) > 0 {
		dimensions = len(embeddings[0])
	}

	l.mu.Lock()
	if decoded.Model != "" {
		l.model = decoded.Model
	}
	if dimensions > 0 {
		l.dimensions = dimensions
	}
	l.mu.Unlock()

	return embeddings, nil
}

// Dimensions returns the last known embedding size.
func (l *LocalEmbedder) Dimensions() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.dimensions
}

// Model returns the configured or last observed model name.
func (l *LocalEmbedder) Model() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.model == "" {
		return "local"
	}
	return l.model
}

type localEmbedRequest struct {
	Inputs []string `json:"inputs"`
	Input  []string `json:"input"`
}

type localEmbedResponse struct {
	Embeddings [][]float32       `json:"embeddings"`
	Data       []localEmbedDatum `json:"data"`
	Dimensions int               `json:"dimensions"`
	Model      string            `json:"model"`
	Error      string            `json:"error"`
}

type localEmbedDatum struct {
	Embedding []float32 `json:"embedding"`
}
