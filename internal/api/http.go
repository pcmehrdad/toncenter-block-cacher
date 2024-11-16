// File: internal/api/http.go

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"toncenter-block-cacher/internal/config"

	"github.com/gorilla/mux"
	"toncenter-block-cacher/internal/utils"
)

type BlockResponse struct {
	BlockNumber int             `json:"block_number"`
	Events      json.RawMessage `json:"events,omitempty"`
	Addresses   json.RawMessage `json:"addresses,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type Server struct {
	fs     *utils.FileSystem
	cfg    *config.Config
	router *mux.Router
}

func NewServer(fs *utils.FileSystem, cfg *config.Config) *Server {
	s := &Server{
		fs:     fs,
		cfg:    cfg,
		router: mux.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) GetRouter() *mux.Router {
	return s.router
}

func (s *Server) setupRoutes() {
	s.router.HandleFunc("/blocks/{number}", s.GetBlock).Methods("GET")
	s.router.HandleFunc("/blocks/latest", s.GetLatestBlock).Methods("GET")
	s.router.HandleFunc("/blocks/range", s.GetBlockRange).Methods("GET")

	// Add middleware for logging and recovery
	s.router.Use(loggingMiddleware)
	s.router.Use(recoveryMiddleware)
}

func (s *Server) Start(port string) error {
	log.Printf("Starting HTTP server on port %s", port)
	return http.ListenAndServe(":"+port, s.router)
}

func (s *Server) GetBlock(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	blockNum, err := strconv.Atoi(vars["number"])
	if err != nil {
		sendError(w, "Invalid block number", http.StatusBadRequest)
		return
	}

	events, addresses, err := s.readBlockData(blockNum)
	if err != nil {
		sendError(w, fmt.Sprintf("Block %d not found", blockNum), http.StatusNotFound)
		return
	}

	response := BlockResponse{
		BlockNumber: blockNum,
		Events:      events,
		Addresses:   addresses,
	}

	sendJSON(w, response)
}

func (s *Server) GetLatestBlock(w http.ResponseWriter, r *http.Request) {
	lastBlock, err := s.fs.GetLastSavedBlockNumber()
	if err != nil {
		sendError(w, "Error getting latest block", http.StatusInternalServerError)
		return
	}

	events, addresses, err := s.readBlockData(int(lastBlock))
	if err != nil {
		sendError(w, "Latest block data not available", http.StatusNotFound)
		return
	}

	response := BlockResponse{
		BlockNumber: int(lastBlock),
		Events:      events,
		Addresses:   addresses,
	}

	sendJSON(w, response)
}

func (s *Server) GetBlockRange(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	start, err := strconv.Atoi(startStr)
	if err != nil {
		sendError(w, "Invalid start block number", http.StatusBadRequest)
		return
	}

	end, err := strconv.Atoi(endStr)
	if err != nil {
		sendError(w, "Invalid end block number", http.StatusBadRequest)
		return
	}

	if end < start {
		sendError(w, "End block must be greater than or equal to start block", http.StatusBadRequest)
		return
	}

	// Use config for range limit
	if end-start > s.cfg.HTTPMaxRangeSize {
		sendError(w, fmt.Sprintf("Range too large. Maximum range is %d blocks", s.cfg.HTTPMaxRangeSize), http.StatusBadRequest)
		return
	}

	var responses []BlockResponse
	for blockNum := start; blockNum <= end; blockNum++ {
		events, addresses, err := s.readBlockData(blockNum)
		if err != nil {
			continue // Skip blocks that don't exist
		}

		responses = append(responses, BlockResponse{
			BlockNumber: blockNum,
			Events:      events,
			Addresses:   addresses,
		})
	}

	if len(responses) == 0 {
		sendError(w, "No blocks found in specified range", http.StatusNotFound)
		return
	}

	sendJSON(w, responses)
}

func (s *Server) readBlockData(blockNum int) (json.RawMessage, json.RawMessage, error) {
	eventsPath := fmt.Sprintf("%s/%d/events.txt", s.fs.GetBasePath(), blockNum)
	addressesPath := fmt.Sprintf("%s/%d/address_book.txt", s.fs.GetBasePath(), blockNum)

	events, err := os.ReadFile(eventsPath)
	if err != nil {
		return nil, nil, err
	}

	addresses, err := os.ReadFile(addressesPath)
	if err != nil {
		return nil, nil, err
	}

	return events, addresses, nil
}

// Helper functions for HTTP responses
func sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	response := BlockResponse{Error: message}
	json.NewEncoder(w).Encode(response)
}

// Middleware for logging
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Middleware for panic recovery
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
