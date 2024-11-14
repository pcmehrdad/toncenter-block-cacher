package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type FileSystem struct {
	basePath string
}

func NewFileSystem(basePath string) *FileSystem {
	return &FileSystem{basePath: basePath}
}

func (fs *FileSystem) SaveBlockData(blockNum int, events []byte, addresses []byte) error {
	dirPath := filepath.Join(fs.basePath, strconv.Itoa(blockNum))
	tempDirPath := dirPath + "_temp"

	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tempDirPath, "events.txt"), events, 0644); err != nil {
		os.RemoveAll(tempDirPath)
		return fmt.Errorf("save events: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tempDirPath, "address_book.txt"), addresses, 0644); err != nil {
		os.RemoveAll(tempDirPath)
		return fmt.Errorf("save addresses: %w", err)
	}

	os.RemoveAll(dirPath)
	return os.Rename(tempDirPath, dirPath)
}

func (fs *FileSystem) GetBlocksToProcess(startBlock, endBlock int) []int {
	files, err := os.ReadDir(fs.basePath)
	if err != nil {
		blocks := make([]int, 0, endBlock-startBlock+1)
		for i := startBlock; i <= endBlock; i++ {
			blocks = append(blocks, i)
		}
		return blocks
	}

	existingBlocks := make(map[int]bool)
	for _, file := range files {
		if file.IsDir() {
			block, err := strconv.Atoi(file.Name())
			if err != nil {
				continue
			}
			blockPath := filepath.Join(fs.basePath, file.Name())
			if IsValidBlockData(blockPath) {
				existingBlocks[block] = true
			}
		}
	}

	var blocksToProcess []int
	for i := startBlock; i <= endBlock; i++ {
		if !existingBlocks[i] {
			blocksToProcess = append(blocksToProcess, i)
		}
	}

	return blocksToProcess
}
