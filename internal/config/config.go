package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// API Configuration
	ApiTonCenterKEY     string
	ApiTonCenterBaseURL string
	ApiTonCenterRPS     int
	ApiFetchLimit       int

	// Storage Configuration
	BlocksSavePath string
	MaxSavedBlocks int

	// Processing Configuration
	MaxParallelFetches int
	DelayOnRetry       time.Duration
	DelayOnBlocks      time.Duration

	// HTTP Server Configuration
	HTTPHost         string
	HTTPPort         string
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPMaxRangeSize int
}

func LoadConfig() (*Config, error) {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	return &Config{
		// API Configuration
		ApiTonCenterKEY:     os.Getenv("API_TONCENTER_KEY"),
		ApiTonCenterBaseURL: os.Getenv("API_TONCENTER_BASE_URL"),
		ApiTonCenterRPS:     intEnv("API_TONCENTER_RPS", 1),
		ApiFetchLimit:       intEnv("API_FETCH_LIMIT", 200),

		// Storage Configuration
		BlocksSavePath: os.Getenv("BLOCKS_SAVE_PATH"),
		MaxSavedBlocks: intEnv("MAX_SAVED_BLOCKS", 604800),

		// Processing Configuration
		MaxParallelFetches: intEnv("MAX_PARALLEL_FETCHES", 5),
		DelayOnRetry:       time.Duration(intEnv("DELAY_ON_RETRY", 100)) * time.Millisecond,
		DelayOnBlocks:      time.Duration(intEnv("DELAY_ON_BLOCKS", 10)) * time.Millisecond,

		// HTTP Server Configuration
		HTTPHost:         getEnv("HTTP_HOST", "localhost"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		HTTPReadTimeout:  time.Duration(intEnv("HTTP_READ_TIMEOUT", 30)) * time.Second,
		HTTPWriteTimeout: time.Duration(intEnv("HTTP_WRITE_TIMEOUT", 30)) * time.Second,
		HTTPMaxRangeSize: intEnv("HTTP_MAX_RANGE_SIZE", 10000),
	}, nil
}

// Helper function to fetch int environment variables with default fallback
func intEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// Helper function to fetch string environment variables with default fallback
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
