package models

// Response represents the API response structure
type Response struct {
	Events      []Event                `json:"events"`
	AddressBook map[string]AddressInfo `json:"address_book"`
}

// Event represents a single event from the API
type Event map[string]interface{}

// AddressInfo contains user-friendly address information
type AddressInfo struct {
	UserFriendly string `json:"user_friendly"`
}

// ChunkResponse represents a response for a single chunk of data
type ChunkResponse struct {
	Chunk  Response
	Offset int
	Err    error
}
