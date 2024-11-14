package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func IsValidBlockData(blockDir string) bool {
	eventsPath := filepath.Join(blockDir, "events.txt")
	eventsData, err := os.ReadFile(eventsPath)
	if err != nil {
		return false
	}

	var events []map[string]interface{}
	if err := json.Unmarshal(eventsData, &events); err != nil {
		return false
	}

	addressPath := filepath.Join(blockDir, "address_book.txt")
	addressData, err := os.ReadFile(addressPath)
	if err != nil {
		return false
	}

	var addresses []string
	return json.Unmarshal(addressData, &addresses) == nil
}
