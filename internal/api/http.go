// File: internal/api/http.go

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"toncenter-block-cacher/internal/config"
	"toncenter-block-cacher/internal/utils"
)

type Server struct {
	fs     *utils.FileSystem
	cfg    *config.Config
	router *mux.Router
}

type HTTPResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
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

func (s *Server) setupRoutes() {
	// Add middleware
	s.router.Use(loggingMiddleware)
	s.router.Use(recoveryMiddleware)

	// API routes
	s.router.HandleFunc("/api/block/{number}", s.handleGetBlock).Methods("GET")
	s.router.HandleFunc("/api/blocks/latest", s.handleGetLatestBlock).Methods("GET")
	s.router.HandleFunc("/api/blocks/range", s.handleGetBlockRange).Methods("GET")
	s.router.HandleFunc("/api/health", s.handleHealth).Methods("GET")
}

func (s *Server) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	blockNum, err := strconv.Atoi(vars["number"])
	if err != nil {
		sendError(w, "Invalid block number", http.StatusBadRequest)
		return
	}

	response, err := s.fs.GetBlockData(blockNum)
	if err != nil {
		sendError(w, fmt.Sprintf("Block %d not found", blockNum), http.StatusNotFound)
		return
	}

	sendSuccess(w, map[string]interface{}{
		"block_number": blockNum,
		"data":         response,
	})
}

func (s *Server) handleGetLatestBlock(w http.ResponseWriter, r *http.Request) {
	lastBlock, err := s.fs.GetLastSavedBlockNumber()
	if err != nil {
		sendError(w, "No blocks available", http.StatusNotFound)
		return
	}

	response, err := s.fs.GetBlockData(int(lastBlock))
	if err != nil {
		sendError(w, "Latest block data not available", http.StatusNotFound)
		return
	}

	sendSuccess(w, map[string]interface{}{
		"block_number": lastBlock,
		"data":         response,
	})
}

func (s *Server) handleGetBlockRange(w http.ResponseWriter, r *http.Request) {
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
		sendError(w, "End block must be greater than start block", http.StatusBadRequest)
		return
	}

	if end-start > s.cfg.HTTPMaxRangeSize {
		sendError(w, fmt.Sprintf("Range too large. Maximum range is %d blocks", s.cfg.HTTPMaxRangeSize), http.StatusBadRequest)
		return
	}

	responses, err := s.fs.GetBlockRange(start, end)
	if err != nil {
		sendError(w, fmt.Sprintf("Error fetching blocks: %v", err), http.StatusInternalServerError)
		return
	}

	sendSuccess(w, map[string]interface{}{
		"start_block": start,
		"end_block":   end,
		"blocks":      responses,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
	}{
		Status:    "OK",
		Timestamp: time.Now(),
	}
	sendSuccess(w, health)
}

func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// Middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Started %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("Completed %s %s in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %v", err)
				sendError(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Helper functions
func sendSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HTTPResponse{
		Success: true,
		Data:    data,
	})
}

func sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(HTTPResponse{
		Success: false,
		Error:   message,
	})
}
