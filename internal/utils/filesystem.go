package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type FileSystem struct {
	basePath string
}

func NewFileSystem(basePath string) *FileSystem {
	return &FileSystem{basePath: basePath}
}

func (fs *FileSystem) SaveBlock(blockNum int, data []byte) error {
	filename := filepath.Join(fs.basePath, fmt.Sprintf("%d.json", blockNum))
	return os.WriteFile(filename, data, 0644)
}

func (fs *FileSystem) GetLastBlock() (int, error) {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var maxBlock int
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			continue
		}
		if blockNum > maxBlock {
			maxBlock = blockNum
		}
	}
	return maxBlock, nil
}

func (fs *FileSystem) RemoveOldBlocks(keepLatest int) error {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		return err
	}

	var blocks []int
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			continue
		}
		blocks = append(blocks, blockNum)
	}

	if len(blocks) <= keepLatest {
		return nil
	}

	sort.Ints(blocks)
	for _, block := range blocks[:len(blocks)-keepLatest] {
		if err := os.Remove(filepath.Join(fs.basePath, fmt.Sprintf("%d.json", block))); err != nil {
			return err
		}
	}
	return nil
}

func (fs *FileSystem) IsBlockValid(blockNum int) bool {
	filename := filepath.Join(fs.basePath, fmt.Sprintf("%d.json", blockNum))

	data, err := os.ReadFile(filename)
	if err != nil {
		return false
	}

	// Check if it's a valid JSON and has the expected structure
	var response struct {
		Events      []json.RawMessage      `json:"events"`
		AddressBook map[string]interface{} `json:"address_book"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return false
	}

	// Check if basic structure is present
	return response.Events != nil && response.AddressBook != nil
}

func (fs *FileSystem) ReadBlock(blockNum int) (json.RawMessage, error) {
	filename := filepath.Join(fs.basePath, fmt.Sprintf("%d.json", blockNum))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read block file: %w", err)
	}
	return json.RawMessage(data), nil
}

func (fs *FileSystem) GetAvailableBlocks() ([]int, error) {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	blocks := make([]int, 0, len(files))
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			continue
		}

		// Verify block is valid
		if fs.IsBlockValid(blockNum) {
			blocks = append(blocks, blockNum)
		}
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no valid blocks found")
	}

	sort.Ints(blocks)
	return blocks, nil
}
