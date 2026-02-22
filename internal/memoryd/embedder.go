package memoryd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	Model() string
}

type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultOllamaURL
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultEmbeddingModel
	}
	return &OllamaEmbedder{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 4 * time.Second},
	}
}

func (o *OllamaEmbedder) Model() string { return o.model }

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	payload := map[string]any{
		"model": o.model,
		"input": text,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	var out struct {
		Embedding  []float64   `json:"embedding"`
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embedding) > 0 {
		return out.Embedding, nil
	}
	if len(out.Embeddings) > 0 && len(out.Embeddings[0]) > 0 {
		return out.Embeddings[0], nil
	}
	return nil, fmt.Errorf("ollama embed response missing embedding")
}
