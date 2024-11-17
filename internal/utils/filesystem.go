package utils

import (
	"encoding/json"
	"fmt"
	"math"
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

	// Pre-allocate slice with 2 capacity since we only need min and max
	blocks := make([]int, 0, 2)
	minBlock, maxBlock := math.MaxInt, math.MinInt

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			continue
		}

		if blockNum < minBlock {
			minBlock = blockNum
		}
		if blockNum > maxBlock {
			maxBlock = blockNum
		}
	}

	if maxBlock == math.MinInt {
		return nil, fmt.Errorf("no valid blocks found")
	}

	// Only append min and max to satisfy the interface
	blocks = append(blocks, minBlock)
	if maxBlock != minBlock {
		blocks = append(blocks, maxBlock)
	}

	return blocks, nil
}
func (fs *FileSystem) GetBlockGaps(start, end int) ([]int, error) {
	var missing []int

	// Create a map of existing blocks
	existing := make(map[int]bool)
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			blockNum, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".json"))
			if err != nil {
				continue
			}
			if blockNum >= start && blockNum <= end {
				existing[blockNum] = true
			}
		}
	}

	// Find missing blocks
	for blockNum := start; blockNum <= end; blockNum++ {
		if !existing[blockNum] {
			missing = append(missing, blockNum)
		}
	}

	return missing, nil
}

func (fs *FileSystem) BlockExists(blockNum int) bool {
	filename := fmt.Sprintf("%d.json", blockNum)
	_, err := os.Stat(filepath.Join(fs.basePath, filename))
	return err == nil
}
