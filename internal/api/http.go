package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"toncenter-block-cacher/internal/utils"
)

type Server struct {
	fs        *utils.FileSystem
	lastBlock int
	mu        sync.RWMutex
}

func NewServer(fs *utils.FileSystem) *Server {
	return &Server{
		fs: fs,
	}
}

func (s *Server) UpdateLastBlock(blockNum int) {
	s.mu.Lock()
	if blockNum > s.lastBlock {
		s.lastBlock = blockNum
	}
	s.mu.Unlock()
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// Get specific block
	mux.HandleFunc("/blocks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		blockID := parts[2]

		// Handle specific block
		blockNum, err := strconv.Atoi(blockID)
		if err != nil {
			http.Error(w, "Invalid block number", http.StatusBadRequest)
			return
		}

		data, err := s.fs.ReadBlock(blockNum)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading block %d: %v", blockNum, err), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	// Get range of blocks
	mux.HandleFunc("/blocks/range", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		startStr := r.URL.Query().Get("start")
		endStr := r.URL.Query().Get("end")

		start, err := strconv.Atoi(startStr)
		if err != nil {
			http.Error(w, "Invalid start block", http.StatusBadRequest)
			return
		}

		end, err := strconv.Atoi(endStr)
		if err != nil {
			http.Error(w, "Invalid end block", http.StatusBadRequest)
			return
		}

		if end < start {
			http.Error(w, "End block must be greater than start block", http.StatusBadRequest)
			return
		}

		if end-start > 50 { // Limit range size
			http.Error(w, "Range too large (maximum 50 blocks)", http.StatusBadRequest)
			return
		}

		blocks := make([]json.RawMessage, 0, end-start+1)
		for i := start; i <= end; i++ {
			data, err := s.fs.ReadBlock(i)
			if err != nil {
				continue // Skip missing blocks
			}
			blocks = append(blocks, data)
		}

		response := map[string]interface{}{
			"blocks": blocks,
			"count":  len(blocks),
			"start":  start,
			"end":    end,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Get available blocks
	mux.HandleFunc("/blocks/available", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		blocks, err := s.fs.GetAvailableBlocks()
		if err != nil {
			http.Error(w, "Error getting available blocks", http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"blocks": blocks,
			"count":  len(blocks),
			"first":  blocks[0],
			"last":   blocks[len(blocks)-1],
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	return http.ListenAndServe(addr, mux)
}
