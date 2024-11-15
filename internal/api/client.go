package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"toncenter-block-cacher/internal/models"
	"toncenter-block-cacher/internal/utils"
)

type Client struct {
	baseURL      string
	apiKeys      []string
	rateLimiters []*utils.RateLimiter
	currentKey   int
	mu           sync.Mutex
}

func NewClient(baseURL string, apiKeys []string, rateLimitPerKey int) *Client {
	rateLimiters := make([]*utils.RateLimiter, len(apiKeys))
	for i := range apiKeys {
		rateLimiters[i] = utils.NewRateLimiter(rateLimitPerKey)
	}

	return &Client{
		baseURL:      baseURL,
		apiKeys:      apiKeys,
		rateLimiters: rateLimiters,
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

func (c *Client) FetchChunk(mcSeqno, limit, offset int) (*models.ChunkResponse, error) {
	apiKey, rateLimiter := c.getNextKey()
	rateLimiter.Wait()

	url := fmt.Sprintf("%s?api_key=%s&mc_seqno=%d&limit=%d&offset=%d&sort=asc",
		c.baseURL, apiKey, mcSeqno, limit, offset)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response failed: %v", err)
	}

	var chunk models.Response
	if err := json.Unmarshal(body, &chunk); err != nil {
		return nil, fmt.Errorf("JSON unmarshaling failed: %v", err)
	}

	return &models.ChunkResponse{
		Chunk:  chunk,
		Offset: offset,
	}, nil
}
