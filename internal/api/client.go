package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"ton-block-processor/internal/models"
	"ton-block-processor/internal/utils"
)

type Client struct {
	baseURL     string
	apiKey      string
	rateLimiter *utils.RateLimiter
}

func NewClient(baseURL, apiKey string, rateLimit int) *Client {
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		rateLimiter: utils.NewRateLimiter(rateLimit),
	}
}

func (c *Client) FetchChunk(mcSeqno, limit, offset int) (*models.ChunkResponse, error) {
	c.rateLimiter.Wait()

	url := fmt.Sprintf("%s?api_key=%s&mc_seqno=%d&limit=%d&offset=%d&sort=asc",
		c.baseURL, c.apiKey, mcSeqno, limit, offset)

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
