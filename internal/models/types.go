package models

type Response struct {
	Events      []Event            `json:"events"`
	AddressBook map[string]Address `json:"address_book"`
}

type Event struct {
	BlockNumber uint32                 `json:"block_number"`
	Action      map[string]interface{} `json:"action"`
	// Add other fields as needed
}

type Address struct {
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	IsScam   bool   `json:"is_scam"`
	Explorer string `json:"explorer"`
}

type ChunkResponse struct {
	Chunk  Response
	Offset int
}
