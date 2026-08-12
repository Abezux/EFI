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
	AIHeadline  string            `json:"ai_headline"`
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
			Timeout: 60 * time.Second,
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

// EnrichEvent synthesizes all source reports into a complete multi-paragraph news report,
// produces a non-truncated headline, selects a category, and extracts named entities for a stable event.
func (g *GeminiLLMClient) EnrichEvent(ctx context.Context, eventTexts []string, validCategories []string) (*EnrichmentResult, error) {
	if len(eventTexts) == 0 {
		return nil, errors.New("cannot enrich event with zero source texts")
	}

	sourcesBlock := strings.Join(eventTexts, "\n\n---\n\n")
	catsList := strings.Join(validCategories, ", ")

	prompt := fmt.Sprintf(`You are an elite, objective economic and financial news journalist and editor for an Ethiopian market intelligence platform.
Using ONLY the facts, figures, and direct statements present in the source texts below (reported by %d source channel post(s) for this single event), write a complete, well-organized, comprehensive news report in flowing English prose.

CORE EDITORIAL & ACCURACY REQUIREMENTS:
1. "ai_headline": Generate a complete, polished, non-truncated news headline in English. It must express a full thought, use proper headline capitalization, and NEVER cut off mid-sentence or end with an ellipsis.
2. "ai_summary": Write a complete, comprehensive news report in flowing English prose — use multiple paragraphs (separated by \n\n) if the source material contains enough corroborated detail.
   - Flow & Structure: Write a cohesive, flowing journalistic narrative rather than disconnected bullet points or a thin blurb.
   - Verbatim Quotations: If any source report includes a direct quote, official statement, or remark, PRESERVE IT VERBATIM and explicitly attribute it to its source channel (e.g., "quoted remark," per Capital Ethiopia).
   - STRICT FACTUALITY & ANTI-HALLUCINATION: Report ONLY facts, numbers, dates, claims, and entity names explicitly present in the provided source texts. Do NOT invent, assume, extrapolate, or add background details not present in the sources.
3. "category": Choose EXACTLY ONE category from this valid list: [%s].
4. "entities": Extract key named entities (people, places, organizations) mentioned in the source reports. For each entity, specify "name" (as referenced in text) and "type" ("person", "place", or "organization").
5. If sources are in Amharic, accurately translate and synthesize into English while preserving entity names and direct quotations.

Source Reports:
%s

Respond STRICTLY in JSON with this exact schema:
{
  "ai_headline": "Complete, non-truncated professional headline in English",
  "ai_summary": "Comprehensive multi-paragraph report in English (paragraphs separated by \\n\\n)...",
  "category": "one exact category from the list",
  "entities": [
    {"name": "Entity Name", "type": "person" | "place" | "organization"}
  ]
}`, len(eventTexts), catsList, sourcesBlock)

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
