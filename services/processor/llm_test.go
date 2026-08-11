package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockLLMClient implements LLMClient for deterministic unit testing.
type MockLLMClient struct {
	VerifyFunc func(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error)
	EnrichFunc func(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error)
	Model      string
}

func (m *MockLLMClient) VerifySameEvent(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error) {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(ctx, postText, candidateEventText)
	}
	return nil, errors.New("VerifyFunc not defined")
}

func (m *MockLLMClient) EnrichEvent(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
	if m.EnrichFunc != nil {
		return m.EnrichFunc(ctx, eventTexts, validCategories)
	}
	return nil, errors.New("EnrichFunc not defined")
}

func (m *MockLLMClient) ModelName() string {
	if m.Model != "" {
		return m.Model
	}
	return "mock-gemini-flash"
}

func TestGeminiLLMClient_VerifySameEvent_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		mockResp := geminiGenerateResponse{
			Candidates: []geminiCandidate{
				{
					Content: struct {
						Parts []geminiPartGen `json:"parts"`
					}{
						Parts: []geminiPartGen{
							{
								Text: `{"decision": "same_event", "confidence": 0.95, "reasoning": "Both texts report NBE raising bank capital."}`,
							},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	client := NewGeminiLLMClient("test-key")
	client.baseURL = ts.URL

	res, err := client.VerifySameEvent(context.Background(), "Post text", "Event text")
	if err != nil {
		t.Fatalf("VerifySameEvent returned error: %v", err)
	}

	if res.Decision != "same_event" {
		t.Errorf("expected decision 'same_event', got %q", res.Decision)
	}
	if res.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", res.Confidence)
	}
	if res.Reasoning == "" {
		t.Errorf("expected reasoning to be populated")
	}
}

func TestGeminiLLMClient_EnrichEvent_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockResp := geminiGenerateResponse{
			Candidates: []geminiCandidate{
				{
					Content: struct {
						Parts []geminiPartGen `json:"parts"`
					}{
						Parts: []geminiPartGen{
							{
								Text: `{
									"ai_summary": "The National Bank of Ethiopia raised minimum capital for commercial banks to 5 billion birr.",
									"category": "Banking & Finance",
									"entities": [
										{"name": "National Bank of Ethiopia", "type": "organization"},
										{"name": "Mamo Mihretu", "type": "person"}
									]
								}`,
							},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	client := NewGeminiLLMClient("test-key")
	client.baseURL = ts.URL

	res, err := client.EnrichEvent(context.Background(), []string{"Text 1", "Text 2"}, []string{"Banking & Finance", "Forex & Trade"})
	if err != nil {
		t.Fatalf("EnrichEvent returned error: %v", err)
	}

	if res.Category != "Banking & Finance" {
		t.Errorf("expected category 'Banking & Finance', got %q", res.Category)
	}
	if len(res.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(res.Entities))
	}
	if res.Entities[0].Name != "National Bank of Ethiopia" || res.Entities[0].Type != "organization" {
		t.Errorf("unexpected entity: %+v", res.Entities[0])
	}
}
