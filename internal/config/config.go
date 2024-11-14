package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIKey              string
	BaseURL             string
	SavePath            string
	TotalRateLimit      int
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

	rateLimit, _ := strconv.Atoi(getEnvWithDefault("TOTAL_RATE_LIMIT", "8"))
	limitOffset, _ := strconv.Atoi(getEnvWithDefault("LIMIT_OFFSET", "500"))
	maxRequests, _ := strconv.Atoi(getEnvWithDefault("MAX_PARALLEL_REQUESTS", "2"))
	maxBlocks, _ := strconv.Atoi(getEnvWithDefault("MAX_PARALLEL_BLOCKS", "5"))
	chunkSize, _ := strconv.Atoi(getEnvWithDefault("CHUNK_SIZE", "1000"))
	retryDelay, _ := strconv.Atoi(getEnvWithDefault("RETRY_DELAY", "1"))
	blockDelay, _ := strconv.Atoi(getEnvWithDefault("BETWEEN_BLOCK_DELAY", "100"))

	return &Config{
		APIKey:              os.Getenv("TON_API_KEY"),
		BaseURL:             os.Getenv("TON_API_BASE_URL"),
		SavePath:            os.Getenv("SAVE_PATH_FOR_BLOCKS"),
		TotalRateLimit:      rateLimit,
		LimitOffset:         limitOffset,
		MaxParallelRequests: maxRequests,
		MaxParallelBlocks:   maxBlocks,
		ChunkSize:           chunkSize,
		RetryDelay:          time.Duration(retryDelay) * time.Second,
		BetweenBlockDelay:   time.Duration(blockDelay) * time.Millisecond,
	}, nil
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
