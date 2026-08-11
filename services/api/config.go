package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
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

// Config encapsulates configuration parameters for the API service.
type Config struct {
	Port               int
	DatabaseURL        string
	RateLimitRPS       float64
	RateLimitBurst     int
	CORSAllowedOrigins string
	MaxPaginationLimit int
	LogLevel           string
	Environment        string
}

// LoadConfig reads configuration from environment variables with safe defaults.
func LoadConfig() (*Config, error) {
	loadDotEnv()

	port := 8080
	if val := os.Getenv("API_PORT"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			port = p
		}
	}

	dbURL := os.Getenv("API_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("APP_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://efi_api:efi_api_pass@localhost:5432/efi_dev?sslmode=disable"
	}

	rateLimitRPS := 10.0
	if val := os.Getenv("RATE_LIMIT_RPS"); val != "" {
		if r, err := strconv.ParseFloat(val, 64); err == nil && r > 0 {
			rateLimitRPS = r
		}
	}

	rateLimitBurst := 20
	if val := os.Getenv("RATE_LIMIT_BURST"); val != "" {
		if b, err := strconv.Atoi(val); err == nil && b > 0 {
			rateLimitBurst = b
		}
	}

	corsOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "*"
	}

	maxLimit := 50
	if val := os.Getenv("MAX_PAGINATION_LIMIT"); val != "" {
		if m, err := strconv.Atoi(val); err == nil && m > 0 {
			maxLimit = m
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

	if dbURL == "" {
		return nil, fmt.Errorf("no database URL configured for API service")
	}

	return &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		RateLimitRPS:       rateLimitRPS,
		RateLimitBurst:     rateLimitBurst,
		CORSAllowedOrigins: corsOrigins,
		MaxPaginationLimit: maxLimit,
		LogLevel:           logLevel,
		Environment:        env,
	}, nil
}
