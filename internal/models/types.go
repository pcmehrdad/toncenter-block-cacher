// File: internal/models/types.go

package models

type Event struct {
	BlockNumber uint32                 `json:"block_number"`
	Action      map[string]interface{} `json:"action"`
}

type AddressInfo struct {
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	IsScam   bool   `json:"is_scam"`
	Explorer string `json:"explorer"`
}

type Response struct {
	Events      []Event                `json:"events"`
	AddressBook map[string]AddressInfo `json:"address_book"`
}

type ChunkResponse struct {
	Chunk  Response
	Offset int
}
