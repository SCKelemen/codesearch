package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalEmbedderEmbedSuccessAndMetadata(t *testing.T) {
	t.Parallel()

	requestCh := make(chan localEmbedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var req localEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requestCh <- req

		if err := json.NewEncoder(w).Encode(localEmbedResponse{
			Embeddings: [][]float32{{1, 2}, {3, 4}},
			Dimensions: 2,
			Model:      "tiny-local",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder := NewLocalEmbedder(server.URL)
	inputs := []string{"hello", "world"}
	vectors, err := embedder.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	want := [][]float32{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(vectors, want) {
		t.Fatalf("vectors = %v, want %v", vectors, want)
	}

	req := <-requestCh
	if !reflect.DeepEqual(req.Inputs, inputs) {
		t.Fatalf("request Inputs = %v, want %v", req.Inputs, inputs)
	}
	if !reflect.DeepEqual(req.Input, inputs) {
		t.Fatalf("request Input = %v, want %v", req.Input, inputs)
	}
	if embedder.Dimensions() != 2 {
		t.Fatalf("Dimensions = %d, want 2", embedder.Dimensions())
	}
	if embedder.Model() != "tiny-local" {
		t.Fatalf("Model = %q, want tiny-local", embedder.Model())
	}

	errCh := make(chan error, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := embedder.Dimensions(); got != 2 {
				errCh <- fmt.Errorf("Dimensions = %d, want 2", got)
			}
			if got := embedder.Model(); got != "tiny-local" {
				errCh <- fmt.Errorf("Model = %q, want tiny-local", got)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestLocalEmbedderEmbedUsesDataFieldAndDefaultModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(localEmbedResponse{
			Data: []localEmbedDatum{{Embedding: []float32{9, 8, 7}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder := NewLocalEmbedder(server.URL)
	vectors, err := embedder.Embed(context.Background(), []string{"only"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	want := [][]float32{{9, 8, 7}}
	if !reflect.DeepEqual(vectors, want) {
		t.Fatalf("vectors = %v, want %v", vectors, want)
	}
	if embedder.Dimensions() != 3 {
		t.Fatalf("Dimensions = %d, want 3", embedder.Dimensions())
	}
	if embedder.Model() != "local" {
		t.Fatalf("Model = %q, want local", embedder.Model())
	}
}

func TestLocalEmbedderEmbedErrorsAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty inputs short circuit", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		vectors, err := embedder.Embed(context.Background(), nil)
		if err != nil {
			t.Fatalf("Embed returned error: %v", err)
		}
		if vectors != nil {
			t.Fatalf("vectors = %v, want nil", vectors)
		}
		if calls.Load() != 0 {
			t.Fatalf("server calls = %d, want 0", calls.Load())
		}
	})

	t.Run("http status failure", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "embedding backend exploded", http.StatusBadGateway)
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		_, err := embedder.Embed(context.Background(), []string{"boom"})
		if err == nil || !strings.Contains(err.Error(), "502 Bad Gateway") || !strings.Contains(err.Error(), "embedding backend exploded") {
			t.Fatalf("Embed error = %v, want status and body", err)
		}
	})

	t.Run("response error field", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if err := json.NewEncoder(w).Encode(localEmbedResponse{Error: "rate limited"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		_, err := embedder.Embed(context.Background(), []string{"boom"})
		if err == nil || !strings.Contains(err.Error(), "rate limited") {
			t.Fatalf("Embed error = %v, want rate limited", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		_, err := embedder.Embed(context.Background(), []string{"boom"})
		if err == nil || !strings.Contains(err.Error(), "decode local embed response") {
			t.Fatalf("Embed error = %v, want decode error", err)
		}
	})

	t.Run("mismatched embedding count", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if err := json.NewEncoder(w).Encode(localEmbedResponse{Embeddings: [][]float32{{1, 2}}}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		_, err := embedder.Embed(context.Background(), []string{"a", "b"})
		if err == nil || !strings.Contains(err.Error(), "returned 1 embeddings for 2 inputs") {
			t.Fatalf("Embed error = %v, want mismatched count", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(250 * time.Millisecond):
				_ = json.NewEncoder(w).Encode(localEmbedResponse{Embeddings: [][]float32{{1}}})
			case <-r.Context().Done():
			}
		}))
		defer server.Close()

		embedder := NewLocalEmbedder(server.URL)
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		_, err := embedder.Embed(ctx, []string{"slow"})
		if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			t.Fatalf("Embed error = %v, want context deadline exceeded", err)
		}
	})
}
