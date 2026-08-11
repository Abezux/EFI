package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		delta    float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 2.0, 3.0},
			b:        []float32{1.0, 2.0, 3.0},
			expected: 1.0,
			delta:    1e-5,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.0, 1.0},
			expected: 0.0,
			delta:    1e-5,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 2.0},
			b:        []float32{-1.0, -2.0},
			expected: -1.0,
			delta:    1e-5,
		},
		{
			name:     "arbitrary angle",
			a:        []float32{1.0, 1.0},
			b:        []float32{1.0, 0.0},
			expected: 1.0 / math.Sqrt(2.0),
			delta:    1e-5,
		},
		{
			name:     "empty or mismatched length",
			a:        []float32{1.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
			delta:    1e-5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CosineSimilarity(tc.a, tc.b)
			if math.Abs(got-tc.expected) > tc.delta {
				t.Errorf("CosineSimilarity(%v, %v) = %f; expected %f (delta %f)", tc.a, tc.b, got, tc.expected, tc.delta)
			}
		})
	}
}

func TestPgVectorFormattingAndParsing(t *testing.T) {
	original := []float32{0.123456, -0.654321, 0.0, 1.0}
	formatted := FormatPgVector(original)
	expectedPrefix := "[0.123456,-0.654321,0.000000,1.000000]"
	if formatted != expectedPrefix {
		t.Errorf("FormatPgVector() = %s; expected %s", formatted, expectedPrefix)
	}

	parsed, err := ParsePgVector(formatted)
	if err != nil {
		t.Fatalf("ParsePgVector() failed: %v", err)
	}

	if len(parsed) != len(original) {
		t.Fatalf("ParsePgVector() returned length %d, expected %d", len(parsed), len(original))
	}

	for i := range original {
		if math.Abs(float64(parsed[i]-original[i])) > 1e-5 {
			t.Errorf("ParsePgVector element %d mismatch: got %f, expected %f", i, parsed[i], original[i])
		}
	}
}

func TestGeminiEmbedder_Success(t *testing.T) {
	mockValues := make([]float32, 768)
	for i := range mockValues {
		mockValues[i] = float32(i) * 0.001
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-api-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req geminiEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		if len(req.Content.Parts) == 0 || req.Content.Parts[0].Text == "" {
			http.Error(w, "Empty text", http.StatusBadRequest)
			return
		}

		resp := geminiEmbedResponse{}
		resp.Embedding.Values = mockValues
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := &GeminiEmbedder{
		apiKey:     "test-api-key",
		model:      "text-embedding-004",
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	ctx := context.Background()
	vec, err := embedder.Embed(ctx, "Test Ethiopian news text")
	if err != nil {
		t.Fatalf("Embed() unexpected error: %v", err)
	}

	if len(vec) != 768 {
		t.Fatalf("Embed() returned vector length %d, expected 768", len(vec))
	}
}

func TestGeminiEmbedder_Errors(t *testing.T) {
	embedder := &GeminiEmbedder{
		apiKey:     "",
		model:      "text-embedding-004",
		baseURL:    "http://invalid",
		httpClient: http.DefaultClient,
	}

	// Empty text
	_, err := embedder.Embed(context.Background(), "")
	if err == nil {
		t.Error("Embed() with empty text expected error, got nil")
	}

	// Missing API key
	_, err = embedder.Embed(context.Background(), "Some text")
	if err == nil {
		t.Error("Embed() with missing API key expected error, got nil")
	}

	// Server error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":429,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	embedder.apiKey = "valid-key"
	embedder.baseURL = server.URL
	embedder.httpClient = server.Client()

	_, err = embedder.Embed(context.Background(), "Some text")
	if err == nil {
		t.Error("Embed() on 429 expected error, got nil")
	}
}
