// File: internal/api/client.go

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
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("404: block not found")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bad status: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response failed: %w", err)
	}

	var chunk models.Response
	if err := json.Unmarshal(body, &chunk); err != nil {
		return nil, fmt.Errorf("JSON unmarshaling failed: %w", err)
	}

	// Initialize empty maps if they're nil
	if chunk.Events == nil {
		chunk.Events = make([]models.Event, 0)
	}
	if chunk.AddressBook == nil {
		chunk.AddressBook = make(map[string]models.AddressInfo)
	}

	return &models.ChunkResponse{
		Chunk:  chunk,
		Offset: offset,
	}, nil
}
