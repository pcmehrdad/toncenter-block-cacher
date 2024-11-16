package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"toncenter-block-cacher/internal/models"
	"toncenter-block-cacher/internal/utils"
)

type Client struct {
	baseURL      string
	apiKeys      []string
	rateLimiters []*utils.RateLimiter
	currentKey   int
	httpClient   *http.Client
	mu           sync.Mutex
}

func NewClient(baseURL string, apiKeys []string, rateLimit int) *Client {
	// Calculate rate limit per key
	ratePerKey := rateLimit
	if len(apiKeys) > 0 {
		ratePerKey = rateLimit / len(apiKeys)
	}

	// Initialize rate limiters
	rateLimiters := make([]*utils.RateLimiter, len(apiKeys))
	for i := range apiKeys {
		rateLimiters[i] = utils.NewRateLimiter(ratePerKey)
	}

	return &Client{
		baseURL:      baseURL,
		apiKeys:      apiKeys,
		rateLimiters: rateLimiters,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
				MaxConnsPerHost:    100,
			},
		},
	}
}

func (c *Client) getNextKey() (string, *utils.RateLimiter) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.apiKeys[c.currentKey]
	limiter := c.rateLimiters[c.currentKey]
	c.currentKey = (c.currentKey + 1) % len(c.apiKeys)

	return key, limiter
}

func (c *Client) FetchChunk(ctx context.Context, mcSeqno, limit, offset int) (*models.ChunkResponse, error) {
	apiKey, rateLimiter := c.getNextKey()

	// Wait for rate limiter
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-rateLimiter.WaitC():
		// Continue with request
	}

	url := fmt.Sprintf("%s?api_key=%s&mc_seqno=%d&limit=%d&offset=%d&sort=asc",
		c.baseURL, apiKey, mcSeqno, limit, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response failed: %w", err)
	}

	// Handle different status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Continue processing
	case http.StatusNotFound:
		return nil, fmt.Errorf("block %d not found", mcSeqno)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limit exceeded")
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var chunk models.Response
	if err := json.Unmarshal(body, &chunk); err != nil {
		return nil, fmt.Errorf("JSON unmarshaling failed: %w", err)
	}

	// Initialize empty slices/maps if nil
	if chunk.Events == nil {
		chunk.Events = make([]models.Event, 0)
	}
	if chunk.AddressBook == nil {
		chunk.AddressBook = make(map[string]models.Address)
	}

	return &models.ChunkResponse{
		Chunk:  chunk,
		Offset: offset,
	}, nil
}

func (c *Client) Stop() {
	for _, limiter := range c.rateLimiters {
		limiter.Stop()
	}
}
