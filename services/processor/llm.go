package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VerificationResult represents the output of a narrow same-event classification call.
type VerificationResult struct {
	Decision    string  `json:"decision"`   // "same_event" | "different_event"
	Confidence  float32 `json:"confidence"` // 0.0 to 1.0
	Reasoning   string  `json:"reasoning"`
	RawResponse string  `json:"-"`
}

// ExtractedEntity represents a named entity extracted during event enrichment.
type ExtractedEntity struct {
	Name string `json:"name"`
	Type string `json:"type"` // "person" | "place" | "organization"
}

// EnrichmentResult represents the output of an event enrichment call.
type EnrichmentResult struct {
	AISummary   string            `json:"ai_summary"`
	Category    string            `json:"category"`
	Entities    []ExtractedEntity `json:"entities"`
	RawResponse string            `json:"-"`
}

// LLMClient defines the contract for text verification and event enrichment calls.
type LLMClient interface {
	VerifySameEvent(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error)
	EnrichEvent(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error)
	ModelName() string
}

// GeminiLLMClient implements LLMClient using Google Gemini text generation API.
type GeminiLLMClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewGeminiLLMClient creates a new Gemini LLM client.
func NewGeminiLLMClient(apiKey string) *GeminiLLMClient {
	return NewGeminiLLMClientWithModel(apiKey, "gemini-flash-latest")
}

// NewGeminiLLMClientWithModel creates a new Gemini LLM client with a specified model.
func NewGeminiLLMClientWithModel(apiKey, model string) *GeminiLLMClient {
	if model == "" {
		model = "gemini-flash-latest"
	}
	return &GeminiLLMClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ModelName returns the model identifier string.
func (g *GeminiLLMClient) ModelName() string {
	return g.model
}

type geminiPartGen struct {
	Text string `json:"text"`
}

type geminiContentGen struct {
	Parts []geminiPartGen `json:"parts"`
}

type geminiGenConfig struct {
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
	Temperature      float32 `json:"temperature,omitempty"`
}

type geminiGenerateRequest struct {
	Contents         []geminiContentGen `json:"contents"`
	GenerationConfig geminiGenConfig    `json:"generationConfig"`
}

type geminiCandidate struct {
	Content struct {
		Parts []geminiPartGen `json:"parts"`
	} `json:"content"`
	FinishReason string `json:"finishReason"`
}

type geminiGenerateResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// generateJSON executes a generation request with JSON mode enabled.
func (g *GeminiLLMClient) generateJSON(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("cannot generate with empty prompt")
	}
	if g.apiKey == "" {
		return "", errors.New("gemini api key is not configured")
	}

	modelPath := fmt.Sprintf("models/%s", g.model)
	reqBody := geminiGenerateRequest{
		Contents: []geminiContentGen{
			{
				Parts: []geminiPartGen{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: geminiGenConfig{
			ResponseMimeType: "application/json",
			Temperature:      0.1,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal gemini generate request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", g.baseURL, modelPath, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create gemini generate http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("gemini generate api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini api error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var genResp geminiGenerateResponse
	if err := json.Unmarshal(respBody, &genResp); err != nil {
		return "", fmt.Errorf("unmarshal gemini generate response: %w", err)
	}

	if genResp.Error != nil {
		return "", fmt.Errorf("gemini api returned error: %s (code %d)", genResp.Error.Message, genResp.Error.Code)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini api returned empty candidate parts")
	}

	return strings.TrimSpace(genResp.Candidates[0].Content.Parts[0].Text), nil
}

// VerifySameEvent performs a narrow same-event vs different-event classification.
func (g *GeminiLLMClient) VerifySameEvent(ctx context.Context, postText, candidateEventText string) (*VerificationResult, error) {
	prompt := fmt.Sprintf(`You are a strict, objective news verification engine for Ethiopian news.
Determine whether the Candidate Post describes the exact same underlying real-world news event as the Existing Cluster, or if it represents a different event/development.

Rules:
1. "same_event": Both texts describe the same core event, policy action, announcement, or occurrence (even if written in different languages, paraphrased, or highlighting different details).
2. "different_event": The texts describe different events, distinct companies, unrelated policies, or different timeframes.
3. "confidence": A float between 0.0 (completely unsure) and 1.0 (certain). If ambiguous, provide a lower confidence score (< 0.75).
4. Do not speculate or invent facts.

Existing Cluster:
%s

Candidate Post:
%s

Respond STRICTLY in JSON with this exact schema:
{
  "decision": "same_event" | "different_event",
  "confidence": float,
  "reasoning": "brief 1-2 sentence explanation"
}`, candidateEventText, postText)

	rawText, err := g.generateJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var res VerificationResult
	if err := json.Unmarshal([]byte(rawText), &res); err != nil {
		return nil, fmt.Errorf("unmarshal verification json: %w (raw: %s)", err, rawText)
	}

	res.RawResponse = rawText
	res.Decision = strings.ToLower(strings.TrimSpace(res.Decision))
	return &res, nil
}

// EnrichEvent generates an objective 2-4 sentence summary, selects a category, and extracts named entities for a stable event.
func (g *GeminiLLMClient) EnrichEvent(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
	if len(eventTexts) == 0 {
		return nil, errors.New("cannot enrich event with zero source texts")
	}

	sourcesBlock := strings.Join(eventTexts, "\n---\n")
	catsList := strings.Join(validCategories, ", ")

	prompt := fmt.Sprintf(`You are an objective financial news analyst and editor for an Ethiopian economic intelligence platform.
Synthesize the provided source reports for a single news event.

Requirements:
1. "ai_summary": Write a concise, factual 2 to 4 sentence synthesis in English. It MUST be written in your own words (not copying any single source directly). It must be objective and free of hype.
2. "category": Choose EXACTLY ONE category from this valid list: [%s].
3. "entities": Extract up to 6 key named entities mentioned in the reports. For each entity, specify "name" (as referenced in text) and "type" ("person", "place", or "organization").
4. If sources are in Amharic, translate and summarize accurately into English while preserving entity names.

Source Reports:
%s

Respond STRICTLY in JSON with this exact schema:
{
  "ai_summary": "2-4 sentence summary in English",
  "category": "one exact category from the list",
  "entities": [
    {"name": "Entity Name", "type": "person" | "place" | "organization"}
  ]
}`, catsList, sourcesBlock)

	rawText, err := g.generateJSON(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var res EnrichmentResult
	if err := json.Unmarshal([]byte(rawText), &res); err != nil {
		return nil, fmt.Errorf("unmarshal enrichment json: %w (raw: %s)", err, rawText)
	}

	res.RawResponse = rawText
	return &res, nil
}
