package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ApiTonCenterKEY     string
	ApiTonCenterBaseURL string
	ApiTonCenterRPS     int
	ApiFetchLimit       int
	BlocksSavePath      string
	MaxSavedBlocks      int
	MaxParallelFetches  int
	DelayOnRetry        time.Duration
	DelayOnBlocks       time.Duration
}

func LoadConfig() (*Config, error) {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	// Return the configuration struct with updated variable names and static values
	return &Config{
		ApiTonCenterKEY:     os.Getenv("API_TONCENTER_KEY"),      // TonCenter API key
		ApiTonCenterBaseURL: os.Getenv("API_TONCENTER_BASE_URL"), // TonCenter API base URL
		ApiTonCenterRPS:     intEnv("API_TONCENTER_RPS", 1),      // Rate limit per second for TonCenter API
		ApiFetchLimit:       intEnv("API_FETCH_LIMIT", 200),      // TonCenter API fetch limit per request

		BlocksSavePath:     os.Getenv("BLOCKS_SAVE_PATH"),                                   // Path to save fetched blocks
		MaxSavedBlocks:     intEnv("MAX_SAVED_BLOCKS", 604800),                              // Maximum saved blocks
		MaxParallelFetches: intEnv("MAX_PARALLEL_FETCHES", 1),                               // Maximum parallel fetches
		DelayOnRetry:       time.Duration(intEnv("DELAY_ON_RETRY", 100)) * time.Millisecond, // Delay on retry if failed
		DelayOnBlocks:      time.Duration(intEnv("DELAY_ON_BLOCKS", 10)) * time.Millisecond, // Delay on blocks fetch
	}, nil
}

// Helper function to fetch int environment variables, with a default fallback
func intEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
