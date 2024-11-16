package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// TON API Configuration
	ToncenterAPIKeys []string
	ToncenterBaseURL string
	ToncenterRPS     int
	FetchLimit       int

	// Storage Configuration
	BlocksSavePath string
	MaxSavedBlocks int // How many blocks to keep in history

	// Block Processing Configuration
	MaxParallelFetches int
	BlocksBehindLatest int // How many blocks to stay behind latest for stability
	ChunkSize          int // Size of block chunks for batch processing

	// Timing Configuration
	RetryDelay      time.Duration
	BlockDelay      time.Duration
	MonitorInterval time.Duration // Interval between checking for new blocks

	// HTTP Server Configuration
	HTTPHost         string
	HTTPPort         string
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPMaxRangeSize int // Maximum number of blocks that can be requested in one range query

	// Validation Configuration
	ValidateBlocks bool // Whether to validate blocks after saving
	MaxRetries     int  // Maximum number of retries for failed operations
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	// Parse API Keys (supporting multiple keys)
	apiKeys := strings.Split(os.Getenv("API_TONCENTER_KEYS"), ",")
	// Clean empty strings from apiKeys
	var cleanedAPIKeys []string
	for _, key := range apiKeys {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			cleanedAPIKeys = append(cleanedAPIKeys, trimmed)
		}
	}

	// Calculate RPS per key if multiple keys are provided
	totalRPS := intEnv("API_TONCENTER_RPS", 8)
	rpsPerKey := totalRPS
	if len(cleanedAPIKeys) > 0 {
		rpsPerKey = totalRPS / len(cleanedAPIKeys)
	}

	config := &Config{
		// TON API Configuration
		ToncenterAPIKeys: cleanedAPIKeys,
		ToncenterBaseURL: getEnv("API_TONCENTER_BASE_URL", "https://toncenter.com/api/v3/events"),
		ToncenterRPS:     rpsPerKey,
		FetchLimit:       intEnv("API_FETCH_LIMIT", 500),

		// Storage Configuration
		BlocksSavePath: getEnv("BLOCKS_SAVE_PATH", "./data/blocks"),
		MaxSavedBlocks: intEnv("MAX_SAVED_BLOCKS", 1000),

		// Block Processing Configuration
		MaxParallelFetches: intEnv("MAX_PARALLEL_FETCHES", 5),
		BlocksBehindLatest: intEnv("BLOCKS_BEHIND_LATEST", 2),
		ChunkSize:          intEnv("CHUNK_SIZE", 100),

		// Timing Configuration
		RetryDelay:      time.Duration(intEnv("DELAY_ON_RETRY", 100)) * time.Millisecond,
		BlockDelay:      time.Duration(intEnv("DELAY_ON_BLOCKS", 10)) * time.Millisecond,
		MonitorInterval: time.Duration(intEnv("MONITOR_INTERVAL", 200)) * time.Millisecond,

		// HTTP Server Configuration
		HTTPHost:         getEnv("HTTP_HOST", "localhost"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		HTTPReadTimeout:  time.Duration(intEnv("HTTP_READ_TIMEOUT", 30)) * time.Second,
		HTTPWriteTimeout: time.Duration(intEnv("HTTP_WRITE_TIMEOUT", 30)) * time.Second,
		HTTPMaxRangeSize: intEnv("HTTP_MAX_RANGE_SIZE", 1000),

		// Validation Configuration
		ValidateBlocks: boolEnv("VALIDATE_BLOCKS", true),
		MaxRetries:     intEnv("MAX_RETRIES", 3),
	}

	// Validate configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) validate() error {
	// Ensure we have at least one API key
	if len(c.ToncenterAPIKeys) == 0 {
		return fmt.Errorf("at least one API key must be provided")
	}

	// Ensure positive values for critical settings
	if c.FetchLimit <= 0 {
		return fmt.Errorf("fetch limit must be positive")
	}

	if c.MaxSavedBlocks <= 0 {
		return fmt.Errorf("max saved blocks must be positive")
	}

	if c.MaxParallelFetches <= 0 {
		return fmt.Errorf("max parallel fetches must be positive")
	}

	return nil
}

// Helper function to get integer environment variables
func intEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// Helper function to get string environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Helper function to get boolean environment variables
func boolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}
	return defaultValue
}
