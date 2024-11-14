package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIKeys             []string
	BaseURL             string
	SavePath            string
	StartBlock          int
	EndBlock            int
	TotalRateLimit      int
	RateLimitPerKey     int
	LimitOffset         int
	MaxParallelRequests int
	MaxParallelBlocks   int
	ChunkSize           int
	RetryDelay          time.Duration
	BetweenBlockDelay   time.Duration
}

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, err
	}

	apiKeys := strings.Split(os.Getenv("TONCENTER_API_KEYS"), ",")
	rateLimit, _ := strconv.Atoi(os.Getenv("TOTAL_RATE_LIMIT"))
	rateLimitPerKey := rateLimit
	if len(apiKeys) > 0 {
		rateLimitPerKey = rateLimit / len(apiKeys)
	}

	return &Config{
		APIKeys:             apiKeys,
		BaseURL:             os.Getenv("TON_API_BASE_URL"),
		SavePath:            os.Getenv("SAVE_PATH_FOR_BLOCKS"),
		StartBlock:          intEnv("START_BLOCK", 35119787),
		EndBlock:            intEnv("END_BLOCK", 999999999),
		TotalRateLimit:      rateLimit,
		RateLimitPerKey:     rateLimitPerKey,
		LimitOffset:         intEnv("LIMIT_OFFSET", 500),
		MaxParallelRequests: intEnv("MAX_PARALLEL_REQUESTS", 2),
		MaxParallelBlocks:   intEnv("MAX_PARALLEL_BLOCKS", 5),
		ChunkSize:           intEnv("CHUNK_SIZE", 1000),
		RetryDelay:          time.Duration(intEnv("RETRY_DELAY", 1)) * time.Millisecond,
		BetweenBlockDelay:   time.Duration(intEnv("BETWEEN_BLOCK_DELAY", 100)) * time.Millisecond,
	}, nil
}

func intEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
