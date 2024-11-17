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

func (fs *FileSystem) RemoveOldBlocks(keepLatest int) error {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
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

	// Sort blocks in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(blocks)))

	// Keep only the latest N blocks
	for _, block := range blocks[keepLatest:] {
		filename := filepath.Join(fs.basePath, fmt.Sprintf("%d.json", block))
		if err := os.Remove(filename); err != nil {
			return fmt.Errorf("remove block %d: %w", block, err)
		}
	}

	return nil
}

func (fs *FileSystem) SaveBlock(blockNum int, data []byte) error {
	filename := filepath.Join(fs.basePath, fmt.Sprintf("%d.json", blockNum))
	return os.WriteFile(filename, data, 0644)
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

	var blocks []int
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
			if err == nil {
				blocks = append(blocks, blockNum)
			}
		}
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no valid blocks found")
	}

	sort.Ints(blocks)
	return blocks, nil
}
