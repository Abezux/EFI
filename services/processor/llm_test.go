package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
									"ai_headline": "National Bank of Ethiopia Raises Commercial Bank Minimum Capital Requirement to 5 Billion Birr",
									"ai_summary": "The National Bank of Ethiopia (NBE) has officially issued a directive raising the minimum capital requirement for all commercial banks operating in the country to 5 billion birr.\n\nAccording to central bank leadership, this measure aims to strengthen financial stability and improve risk absorption capacity across the banking system. \"This capitalization mandate will fortify domestic institutions against macroeconomic volatility,\" per Capital Ethiopia.",
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

	if res.AIHeadline != "National Bank of Ethiopia Raises Commercial Bank Minimum Capital Requirement to 5 Billion Birr" {
		t.Errorf("unexpected ai_headline: %q", res.AIHeadline)
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

func TestGeminiLLMClient_EnrichEvent_PromptCompliance(t *testing.T) {
	var capturedPrompt string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Contents) > 0 && len(req.Contents[0].Parts) > 0 {
			capturedPrompt = req.Contents[0].Parts[0].Text
		}

		mockResp := geminiGenerateResponse{
			Candidates: []geminiCandidate{
				{
					Content: struct {
						Parts []geminiPartGen `json:"parts"`
					}{
						Parts: []geminiPartGen{
							{
								Text: `{"ai_headline": "Test Headline", "ai_summary": "Test summary", "category": "Banking & Finance", "entities": []}`,
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

	sources := []string{
		"[Source 1 — Capital Ethiopia (@Capitalethiopia)]\nNBE increased reserve requirements.",
		"[Source 2 — Addis Fortune (@addisfortune)]\nBanks react to the new NBE directives.",
	}
	_, err := client.EnrichEvent(context.Background(), sources, []string{"Banking & Finance"})
	if err != nil {
		t.Fatalf("EnrichEvent error: %v", err)
	}

	// Verify required V4.1 prompt elements per spec
	requiredPhrases := []string{
		"ai_headline",
		"ai_summary",
		"STRICT FACTUALITY & ANTI-HALLUCINATION",
		"Do NOT invent, assume, extrapolate",
		"PRESERVE IT VERBATIM",
		"Capital Ethiopia",
		"Addis Fortune",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(capturedPrompt, phrase) {
			t.Errorf("expected prompt to contain %q, but was missing in:\n%s", phrase, capturedPrompt)
		}
	}
}
