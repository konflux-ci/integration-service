package coverage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/coverage"
	"time"
)

// CoverageResponse represents the JSON response from the coverage endpoint
type CoverageResponse struct {
	MetaFilename     string `json:"meta_filename"`
	MetaData         string `json:"meta_data"` // base64 encoded
	CountersFilename string `json:"counters_filename"`
	CountersData     string `json:"counters_data"` // base64 encoded
	Timestamp        int64  `json:"timestamp"`
}

func init() {
	// Start coverage server in a separate goroutine
	go startCoverageServer()
}

// startCoverageServer starts a dedicated HTTP server for coverage collection
func startCoverageServer() {
	// Get coverage port from environment variable, default to 9095
	coveragePort := os.Getenv("COVERAGE_PORT")
	if coveragePort == "" {
		coveragePort = "9095"
	}

	// Create a new ServeMux for the coverage server (isolated from main app)
	mux := http.NewServeMux()
	mux.HandleFunc("/coverage", CoverageHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "coverage server healthy")
	})

	addr := ":" + coveragePort
	log.Printf("[COVERAGE] Starting coverage server on %s", addr)
	log.Printf("[COVERAGE] Endpoints: GET %s/coverage, GET %s/health", addr, addr)

	// Start the server (this will block, but we're in a goroutine)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("[COVERAGE] ERROR: Coverage server failed: %v", err)
	}
}

// CoverageHandler collects coverage data and returns it via HTTP as JSON
func CoverageHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("[COVERAGE] Collecting coverage data...")

	// Collect metadata
	var metaBuf bytes.Buffer
	if err := coverage.WriteMeta(&metaBuf); err != nil {
		http.Error(w, fmt.Sprintf("Failed to collect metadata: %v", err), http.StatusInternalServerError)
		return
	}
	metaData := metaBuf.Bytes()

	// Collect counters
	var counterBuf bytes.Buffer
	if err := coverage.WriteCounters(&counterBuf); err != nil {
		http.Error(w, fmt.Sprintf("Failed to collect counters: %v", err), http.StatusInternalServerError)
		return
	}
	counterData := counterBuf.Bytes()

	// Extract hash from metadata to create proper filenames
	var hash string
	if len(metaData) >= 32 {
		hashBytes := metaData[16:32]
		hash = fmt.Sprintf("%x", hashBytes)
	} else {
		hash = "unknown"
	}

	// Generate proper filenames
	timestamp := time.Now().UnixNano()
	metaFilename := fmt.Sprintf("covmeta.%s", hash)
	counterFilename := fmt.Sprintf("covcounters.%s.%d.%d", hash, os.Getpid(), timestamp)

	log.Printf("[COVERAGE] Collected %d bytes metadata, %d bytes counters",
		len(metaData), len(counterData))

	// Return coverage data as JSON
	response := CoverageResponse{
		MetaFilename:     metaFilename,
		MetaData:         base64.StdEncoding.EncodeToString(metaData),
		CountersFilename: counterFilename,
		CountersData:     base64.StdEncoding.EncodeToString(counterData),
		Timestamp:        timestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[COVERAGE] Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	log.Println("[COVERAGE] Coverage data sent successfully")
}
