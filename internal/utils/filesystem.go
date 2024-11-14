package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

type FileSystem struct {
	basePath  string
	validator BlockValidator
	cache     struct {
		sync.RWMutex
		blocks map[int]bool
	}
}

func NewFileSystem(basePath string, validator BlockValidator) *FileSystem {
	fs := &FileSystem{
		basePath:  basePath,
		validator: validator,
	}
	fs.cache.blocks = make(map[int]bool)
	return fs
}

func (fs *FileSystem) SaveBlockData(blockNum int, events []byte, addresses []byte) error {
	dirPath := filepath.Join(fs.basePath, strconv.Itoa(blockNum))
	tempDirPath := dirPath + "_temp"

	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Write files concurrently
	var wg sync.WaitGroup
	var eventErr, addrErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		eventErr = os.WriteFile(filepath.Join(tempDirPath, "events.txt"), events, 0644)
	}()

	go func() {
		defer wg.Done()
		addrErr = os.WriteFile(filepath.Join(tempDirPath, "address_book.txt"), addresses, 0644)
	}()

	wg.Wait()

	if eventErr != nil || addrErr != nil {
		os.RemoveAll(tempDirPath)
		if eventErr != nil {
			return fmt.Errorf("save events: %w", eventErr)
		}
		return fmt.Errorf("save addresses: %w", addrErr)
	}

	// Atomic directory replacement
	if err := os.RemoveAll(dirPath); err != nil {
		os.RemoveAll(tempDirPath)
		return fmt.Errorf("remove old dir: %w", err)
	}

	if err := os.Rename(tempDirPath, dirPath); err != nil {
		os.RemoveAll(tempDirPath)
		return fmt.Errorf("rename dir: %w", err)
	}

	// Update cache
	fs.cache.Lock()
	fs.cache.blocks[blockNum] = true
	fs.cache.Unlock()

	return nil
}

func (fs *FileSystem) loadExistingBlocksParallel(paths []string, result map[int]bool) {
	numWorkers := runtime.NumCPU() * 2
	pathChan := make(chan string, len(paths))
	var wg sync.WaitGroup
	var mutex sync.Mutex

	// Fill work channel
	for _, path := range paths {
		pathChan <- path
	}
	close(pathChan)

	// Create worker pool
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathChan {
				if block, err := strconv.Atoi(filepath.Base(path)); err == nil {
					blockPath := filepath.Join(fs.basePath, strconv.Itoa(block))
					if fs.validator.IsValid(blockPath) {
						mutex.Lock()
						result[block] = true
						mutex.Unlock()
					}
				}
			}
		}()
	}

	wg.Wait()
}

func (fs *FileSystem) GetBlocksToProcess(startBlock, endBlock int) []int {
	// Quick path for non-existent directory
	if _, err := os.Stat(fs.basePath); os.IsNotExist(err) {
		blocks := make([]int, 0, endBlock-startBlock+1)
		for i := startBlock; i <= endBlock; i++ {
			blocks = append(blocks, i)
		}
		return blocks
	}

	// Use cache if available
	fs.cache.RLock()
	if len(fs.cache.blocks) > 0 {
		fs.cache.RUnlock()
		return fs.getBlocksFromCache(startBlock, endBlock)
	}
	fs.cache.RUnlock()

	// Read directory entries
	var paths []string
	entries, err := os.ReadDir(fs.basePath)
	if err == nil {
		paths = make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, entry.Name())
			}
		}
	}

	// Process existing blocks in parallel
	existingBlocks := make(map[int]bool)
	fs.loadExistingBlocksParallel(paths, existingBlocks)

	// Update cache
	fs.cache.Lock()
	fs.cache.blocks = existingBlocks
	fs.cache.Unlock()

	// Generate result with pre-allocated capacity
	estimatedSize := endBlock - startBlock + 1
	blocksToProcess := make([]int, 0, estimatedSize)

	for i := startBlock; i <= endBlock; i++ {
		if !existingBlocks[i] {
			blocksToProcess = append(blocksToProcess, i)
		}
	}

	return blocksToProcess
}

func (fs *FileSystem) getBlocksFromCache(startBlock, endBlock int) []int {
	fs.cache.RLock()
	defer fs.cache.RUnlock()

	blocksToProcess := make([]int, 0, endBlock-startBlock+1)
	for i := startBlock; i <= endBlock; i++ {
		if !fs.cache.blocks[i] {
			blocksToProcess = append(blocksToProcess, i)
		}
	}
	return blocksToProcess
}

// ClearCache clears the internal cache if needed
func (fs *FileSystem) ClearCache() {
	fs.cache.Lock()
	fs.cache.blocks = make(map[int]bool)
	fs.cache.Unlock()
}
