package models

import (
	"time"
)

// Response represents the API response structure for TON events
type Response struct {
	Events      []Event            `json:"events"`
	AddressBook map[string]Address `json:"address_book,omitempty"`
}

// Event represents a single event from the TON blockchain
type Event struct {
	EventID   string                 `json:"event_id"`
	Account   string                 `json:"account"`
	Timestamp int64                  `json:"timestamp"`
	Lt        string                 `json:"lt"`
	Type      string                 `json:"type"`
	Value     float64                `json:"value,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Address represents the address book entry
type Address struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Metadata string `json:"metadata,omitempty"`
}

// ChunkResponse wraps a response with additional metadata
type ChunkResponse struct {
	Chunk  Response
	Offset int
	Error  error
}

// MonitorStats holds statistics about the block monitoring process
type MonitorStats struct {
	LastProcessedBlock int       `json:"last_processed_block"`
	ProcessedBlocks    int       `json:"processed_blocks"`
	FailedBlocks       int       `json:"failed_blocks"`
	ProcessingRate     float64   `json:"processing_rate"`
	StartTime          time.Time `json:"start_time"`
	LastError          string    `json:"last_error,omitempty"`
}
