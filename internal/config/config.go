package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// TON API Configuration
	ToncenterAPIKey  string
	ToncenterBaseURL string
	ToncenterRPS     int
	FetchLimit       int

	// Storage Configuration
	BlocksSavePath string
	MaxSavedBlocks int

	// Block Processing Configuration
	MaxParallelFetches int

	// Timing Configuration
	RetryDelay time.Duration
	BlockDelay time.Duration

	// HTTP Server Configuration
	HTTPHost string
	HTTPPort string
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	config := &Config{
		// TON API Configuration
		ToncenterAPIKey:  getEnv("API_TONCENTER_KEY", ""),
		ToncenterBaseURL: getEnv("API_TONCENTER_BASE_URL", "https://toncenter.com/api/v3/events"),
		ToncenterRPS:     intEnv("API_TONCENTER_RPS", 8),
		FetchLimit:       intEnv("API_FETCH_LIMIT", 500),

		// Storage Configuration
		BlocksSavePath: "./data/blocks",
		MaxSavedBlocks: intEnv("MAX_SAVED_BLOCKS", 50) + 20,

		// Block Processing Configuration
		MaxParallelFetches: intEnv("MAX_PARALLEL_FETCHES", 5),

		// Timing Configuration
		RetryDelay: time.Duration(intEnv("DELAY_ON_RETRY", 100)) * time.Millisecond,
		BlockDelay: time.Duration(intEnv("DELAY_ON_BLOCKS", 10)) * time.Millisecond,

		// HTTP Server Configuration
		HTTPHost: "0.0.0.0",
		HTTPPort: "8080",
	}
	return config, nil
}

func intEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
