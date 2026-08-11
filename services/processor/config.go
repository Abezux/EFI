package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config encapsulates configuration parameters for the processor service.
type Config struct {
	DatabaseURL      string
	PollInterval     time.Duration
	BatchSize        int
	SimhashThreshold int
	ClusteringWindow time.Duration
	LogLevel         string
	Environment      string
}

// LoadConfig reads configuration from environment variables with sensible defaults.
func LoadConfig() (*Config, error) {
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

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		DatabaseURL:      dbURL,
		PollInterval:     pollInterval,
		BatchSize:        batchSize,
		SimhashThreshold: simhashThreshold,
		ClusteringWindow: clusteringWindow,
		LogLevel:         logLevel,
		Environment:      env,
	}, nil
}
