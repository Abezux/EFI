package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func loadDotEnv() {
	file, err := os.Open(".env")
	if err != nil {
		file, err = os.Open("../../.env")
		if err != nil {
			return
		}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, "\"'")
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func loadDockerSecrets() {
	secretMap := map[string]string{
		"gemini_api_key":   "GEMINI_API_KEY",
		"app_db_password":  "APP_DB_PASSWORD",
		"postgres_password": "POSTGRES_PASSWORD",
	}
	for secretFile, envVar := range secretMap {
		if os.Getenv(envVar) != "" {
			continue
		}
		data, err := os.ReadFile("/run/secrets/" + secretFile)
		if err == nil {
			val := strings.TrimSpace(string(data))
			if val != "" {
				os.Setenv(envVar, val)
			}
		}
	}
}

// Config encapsulates configuration parameters for the processor service.
type Config struct {
	DatabaseURL            string
	PollInterval           time.Duration
	BatchSize              int
	SimhashThreshold       int
	ClusteringWindow       time.Duration
	GeminiAPIKey           string
	EmbeddingHighThreshold float64
	EmbeddingLowThreshold  float64
	MaxEmbeddingRetries    int

	// V4 AI Enrichment Configuration
	LLMModel                  string
	VerifyConfidenceThreshold float32
	StabilityWindowMinutes    int
	MaxLLMRetries             int
	LogLevel                  string
	Environment               string
}

// LoadConfig reads configuration from environment variables with sensible defaults.
func LoadConfig() (*Config, error) {
	loadDotEnv()
	loadDockerSecrets()
	dbURL := os.Getenv("APP_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return nil, fmt.Errorf("neither APP_DATABASE_URL nor DATABASE_URL is set in environment")
	}

	pollInterval := 2 * time.Second
	if val := os.Getenv("POLL_INTERVAL_SECONDS"); val != "" {
		if sec, err := strconv.Atoi(val); err == nil && sec > 0 {
			pollInterval = time.Duration(sec) * time.Second
		}
	}

	batchSize := 50
	if val := os.Getenv("BATCH_SIZE"); val != "" {
		if bs, err := strconv.Atoi(val); err == nil && bs > 0 {
			batchSize = bs
		}
	}

	// Simhash Hamming distance threshold (0-64 bits). Default 10 per V2 specification.
	simhashThreshold := 10
	if val := os.Getenv("SIMHASH_THRESHOLD"); val != "" {
		if st, err := strconv.Atoi(val); err == nil && st >= 0 && st <= 64 {
			simhashThreshold = st
		}
	}

	// Clustering time window. Default 48 hours per architecture specification.
	clusteringWindow := 48 * time.Hour
	if val := os.Getenv("CLUSTERING_WINDOW_HOURS"); val != "" {
		if cw, err := strconv.Atoi(val); err == nil && cw > 0 {
			clusteringWindow = time.Duration(cw) * time.Hour
		}
	}

	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		geminiAPIKey = os.Getenv("EMBEDDING_API_KEY")
	}

	// High cosine similarity threshold for auto-attaching events (default 0.82)
	embeddingHighThreshold := 0.82
	if val := os.Getenv("EMBEDDING_HIGH_THRESHOLD"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0.0 && f <= 1.0 {
			embeddingHighThreshold = f
		}
	}

	// Low cosine similarity threshold below which a new event is created (default 0.65)
	embeddingLowThreshold := 0.65
	if val := os.Getenv("EMBEDDING_LOW_THRESHOLD"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0.0 && f <= 1.0 {
			embeddingLowThreshold = f
		}
	}

	maxEmbeddingRetries := 3
	if val := os.Getenv("MAX_EMBEDDING_RETRIES"); val != "" {
		if r, err := strconv.Atoi(val); err == nil && r >= 0 {
			maxEmbeddingRetries = r
		}
	}

	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "gemini-flash-latest"
	}

	verifyConfidenceThreshold := float32(0.75)
	if val := os.Getenv("VERIFY_CONFIDENCE_THRESHOLD"); val != "" {
		if f, err := strconv.ParseFloat(val, 32); err == nil && f > 0.0 && f <= 1.0 {
			verifyConfidenceThreshold = float32(f)
		}
	}

	stabilityWindowMinutes := 15
	if val := os.Getenv("STABILITY_WINDOW_MINUTES"); val != "" {
		if sw, err := strconv.Atoi(val); err == nil && sw >= 0 {
			stabilityWindowMinutes = sw
		}
	}

	maxLLMRetries := 3
	if val := os.Getenv("MAX_LLM_RETRIES"); val != "" {
		if r, err := strconv.Atoi(val); err == nil && r >= 0 {
			maxLLMRetries = r
		}
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		DatabaseURL:               dbURL,
		PollInterval:              pollInterval,
		BatchSize:                 batchSize,
		SimhashThreshold:          simhashThreshold,
		ClusteringWindow:          clusteringWindow,
		GeminiAPIKey:              geminiAPIKey,
		EmbeddingHighThreshold:    embeddingHighThreshold,
		EmbeddingLowThreshold:     embeddingLowThreshold,
		MaxEmbeddingRetries:       maxEmbeddingRetries,
		LLMModel:                  llmModel,
		VerifyConfidenceThreshold: verifyConfidenceThreshold,
		StabilityWindowMinutes:    stabilityWindowMinutes,
		MaxLLMRetries:             maxLLMRetries,
		LogLevel:                  logLevel,
		Environment:               env,
	}, nil
}
