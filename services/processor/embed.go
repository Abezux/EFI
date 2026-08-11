package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Embedder defines the contract for generating text embeddings.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dimension() int
}

// GeminiEmbedder implements Embedder using Google Gemini embedding API.
type GeminiEmbedder struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewGeminiEmbedder creates a new Gemini embedding client defaulting to gemini-embedding-2 (768-d).
func NewGeminiEmbedder(apiKey string) *GeminiEmbedder {
	return NewGeminiEmbedderWithModel(apiKey, "gemini-embedding-2")
}

// NewGeminiEmbedderWithModel creates a new Gemini embedding client with a specified model.
func NewGeminiEmbedderWithModel(apiKey, model string) *GeminiEmbedder {
	if model == "" {
		model = "gemini-embedding-2"
	}
	return &GeminiEmbedder{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dimension returns the vector output dimension for the embedding model.
func (g *GeminiEmbedder) Dimension() int {
	return 768
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiEmbedRequest struct {
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Embed generates a 768-dimensional embedding for the input text via Gemini API.
func (g *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("cannot embed empty text")
	}
	if g.apiKey == "" {
		return nil, errors.New("gemini api key is not configured")
	}

	modelPath := fmt.Sprintf("models/%s", g.model)
	reqBody := geminiEmbedRequest{
		Content: geminiContent{
			Parts: []geminiPart{
				{Text: text},
			},
		},
		OutputDimensionality: 768,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini embed request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:embedContent?key=%s", g.baseURL, modelPath, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create gemini embed http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var embedResp geminiEmbedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal gemini embed response: %w", err)
	}

	if embedResp.Error != nil {
		return nil, fmt.Errorf("gemini api returned error: %s (code %d)", embedResp.Error.Message, embedResp.Error.Code)
	}

	if len(embedResp.Embedding.Values) == 0 {
		return nil, errors.New("gemini api returned empty embedding vector")
	}

	return embedResp.Embedding.Values, nil
}

// CosineSimilarity computes the cosine similarity between two float32 slices.
// Returns value between -1.0 and 1.0 (typically 0.0 to 1.0 for normalized embeddings).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		valA := float64(a[i])
		valB := float64(b[i])
		dotProduct += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// FormatPgVector converts a float32 slice into PostgreSQL vector string format "[0.1,0.2,...]".
func FormatPgVector(v []float32) string {
	if len(v) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, val := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(val), 'f', 6, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}

// ParsePgVector parses a PostgreSQL vector string format "[0.1,0.2,...]" into []float32.
func ParsePgVector(s string) ([]float32, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return nil, nil
	}
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	vec := make([]float32, len(parts))
	for i, p := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, fmt.Errorf("parse vector element at %d: %w", i, err)
		}
		vec[i] = float32(val)
	}
	return vec, nil
}
