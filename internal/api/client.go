package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"toncenter-block-cacher/internal/utils"
)

type Client struct {
	baseURL     string
	apiKey      string
	rateLimiter *utils.RateLimiter
}

func NewClient(baseURL, apiKey string, rps int) *Client {
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		rateLimiter: utils.NewRateLimiter(rps),
	}
}

func (c *Client) FetchChunk(mcSeqno, limit, offset int) ([]byte, int, error) {
	// Apply rate limiting
	c.rateLimiter.Wait()

	url := fmt.Sprintf("%s?api_key=%s&mc_seqno=%d&limit=%d&offset=%d&sort=asc",
		c.baseURL, c.apiKey, mcSeqno, limit, offset)

	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading response failed: %w", err)
	}

	// Parse the response to get events count
	var chunk struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(body, &chunk); err != nil {
		return nil, 0, fmt.Errorf("invalid JSON response: %w", err)
	}

	return body, len(chunk.Events), nil
}
