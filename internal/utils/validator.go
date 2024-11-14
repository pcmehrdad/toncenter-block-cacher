package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BlockValidator interface defines the contract for block validation
type BlockValidator interface {
	IsValid(blockPath string) bool
}

// FastBlockValidator performs quick validation by checking file existence
type FastBlockValidator struct{}

func (v *FastBlockValidator) IsValid(blockPath string) bool {
	if _, err := os.Stat(filepath.Join(blockPath, "events.txt")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(blockPath, "address_book.txt")); err != nil {
		return false
	}
	return true
}

// ThoroughBlockValidator performs complete validation including JSON parsing
type ThoroughBlockValidator struct{}

func (v *ThoroughBlockValidator) IsValid(blockPath string) bool {
	// Validate events file
	eventsPath := filepath.Join(blockPath, "events.txt")
	eventsData, err := os.ReadFile(eventsPath)
	if err != nil {
		return false
	}

	var events []map[string]interface{}
	if err := json.Unmarshal(eventsData, &events); err != nil {
		return false
	}

	// Validate address book file
	addressPath := filepath.Join(blockPath, "address_book.txt")
	addressData, err := os.ReadFile(addressPath)
	if err != nil {
		return false
	}

	var addresses map[string]interface{}
	return json.Unmarshal(addressData, &addresses) == nil
}

// For backward compatibility if needed (though better to use the new interface)
var (
	IsValidBlockData     = (&ThoroughBlockValidator{}).IsValid
	IsValidBlockDataFast = (&FastBlockValidator{}).IsValid
)
