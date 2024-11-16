package utils

import (
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

// Helper function for quick validation
func IsValidBlockData(blockPath string) bool {
	return (&FastBlockValidator{}).IsValid(blockPath)
}
