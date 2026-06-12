package coverage

// Compatible with openshift-eng/art-tools coverage_server.go.
// Inspired by: https://github.com/psturc/go-coverage-http
//
// Multiple containers in a pod may each start a coverage server. The server
// tries ports starting at DefaultPort (or COVERAGE_PORT) and increments
// up to MaxRetries times until it finds a free port.
//
// Clients can identify a coverage server by sending a HEAD request to any
// endpoint: the response will include the headers:
//   X-Art-Coverage-Server:         1
//   X-Art-Coverage-Pid:            <pid>
//   X-Art-Coverage-Binary:         <binary-name>
//   X-Art-Coverage-Source-Commit:  <commit>  (if SOURCE_GIT_COMMIT is set)
//   X-Art-Coverage-Source-Url:     <url>     (if SOURCE_GIT_URL is set)
//   X-Art-Coverage-Software-Group: <group>   (if SOFTWARE_GROUP or __doozer_group is set)
//   X-Art-Coverage-Software-Key:   <key>     (if SOFTWARE_KEY or __doozer_key is set)
//
// Pass ?nometa=1 to /coverage to skip metadata collection on subsequent
// requests (metadata does not change for the lifetime of the process).

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/coverage"
	"strconv"
	"sync"
	"time"
)

const (
	DefaultPort = 53700 // Starting port for the coverage server
	MaxRetries  = 50    // Maximum number of ports to try
)

// metaHashOnce ensures the metadata hash is computed exactly once.
// The hash is process-stable so it never changes after the first read.
var (
	metaHashOnce sync.Once
	metaHash     string
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

// identityMiddleware wraps a handler to add identification headers to
// every response (including HEAD requests) so that clients can confirm they
// are talking to a coverage server rather than an unrelated process.
func identityMiddleware(next http.Handler) http.Handler {
	pid := strconv.Itoa(os.Getpid())
	exe := "unknown"
	if exePath, err := os.Executable(); err == nil {
		exe = filepath.Base(exePath)
	}

	softwareGroup := os.Getenv("SOFTWARE_GROUP")
	if softwareGroup == "" {
		softwareGroup = os.Getenv("__doozer_group")
	}
	softwareKey := os.Getenv("SOFTWARE_KEY")
	if softwareKey == "" {
		softwareKey = os.Getenv("__doozer_key")
	}
	envHeaders := map[string]string{
		"X-Art-Coverage-Source-Commit":  os.Getenv("SOURCE_GIT_COMMIT"),
		"X-Art-Coverage-Source-Url":     os.Getenv("SOURCE_GIT_URL"),
		"X-Art-Coverage-Software-Group": softwareGroup,
		"X-Art-Coverage-Software-Key":   softwareKey,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Art-Coverage-Server", "1")
		w.Header().Set("X-Art-Coverage-Pid", pid)
		w.Header().Set("X-Art-Coverage-Binary", exe)
		for header, val := range envHeaders {
			if val != "" {
				w.Header().Set(header, val)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// startCoverageServer starts a dedicated HTTP server for coverage collection.
// It tries successive ports starting from COVERAGE_PORT (default 53700)
// until one is available or MaxRetries attempts are exhausted.
func startCoverageServer() {
	startPort := DefaultPort
	if envPort := os.Getenv("COVERAGE_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			startPort = p
		}
	}

	// Create a new ServeMux for the coverage server (isolated from main app)
	mux := http.NewServeMux()
	mux.HandleFunc("/coverage", CoverageHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "coverage server healthy")
	})

	handler := identityMiddleware(mux)

	for attempt := 0; attempt < MaxRetries; attempt++ {
		port := startPort + attempt
		addr := fmt.Sprintf(":%d", port)

		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("[COVERAGE] Port %d unavailable: %v; trying next", port, err)
			continue
		}

		log.Printf("[COVERAGE] Starting coverage server on port %d (pid %d)", port, os.Getpid())
		log.Printf("[COVERAGE] Endpoints: GET %s/coverage, GET %s/health, HEAD %s/*", addr, addr, addr)

		if err := http.Serve(ln, handler); err != nil {
			log.Printf("[COVERAGE] ERROR: Coverage server on port %d failed: %v", port, err)
		}
		return
	}

	log.Printf("[COVERAGE] ERROR: Could not bind any port in range %d–%d", startPort, startPort+MaxRetries-1)
}

// ensureMetaHash computes and caches the metadata hash exactly once.
// The hash is derived from the binary's coverage metadata which is
// constant for the lifetime of the process.
func ensureMetaHash() {
	metaHashOnce.Do(func() {
		var buf bytes.Buffer
		if err := coverage.WriteMeta(&buf); err != nil {
			log.Printf("[COVERAGE] Warning: could not prime metadata hash: %v", err)
			metaHash = "unknown"
			return
		}
		data := buf.Bytes()
		if len(data) >= 32 {
			metaHash = fmt.Sprintf("%x", data[16:32])
		} else {
			metaHash = "unknown"
		}
	})
}

// CoverageHandler collects coverage data and returns it via HTTP as JSON.
// Pass ?nometa=1 to skip metadata collection (useful after the first fetch
// since metadata does not change for the lifetime of the process).
func CoverageHandler(w http.ResponseWriter, r *http.Request) {
	skipMeta := r.URL.Query().Get("nometa") == "1"

	if skipMeta {
		log.Println("[COVERAGE] Collecting coverage counters (metadata skipped)...")
	} else {
		log.Println("[COVERAGE] Collecting coverage data...")
	}

	// Ensure the hash is primed (safe for concurrent use, runs once)
	ensureMetaHash()

	var metaData []byte
	var metaFilename string
	if !skipMeta {
		var metaBuf bytes.Buffer
		if err := coverage.WriteMeta(&metaBuf); err != nil {
			http.Error(w, fmt.Sprintf("Failed to collect metadata: %v", err), http.StatusInternalServerError)
			return
		}
		metaData = metaBuf.Bytes()
		metaFilename = fmt.Sprintf("covmeta.%s", metaHash)
	}

	// Collect counters
	var counterBuf bytes.Buffer
	if err := coverage.WriteCounters(&counterBuf); err != nil {
		http.Error(w, fmt.Sprintf("Failed to collect counters: %v", err), http.StatusInternalServerError)
		return
	}
	counterData := counterBuf.Bytes()

	// Generate counter filename using the cached hash
	timestamp := time.Now().UnixNano()
	counterFilename := fmt.Sprintf("covcounters.%s.%d.%d", metaHash, os.Getpid(), timestamp)

	log.Printf("[COVERAGE] Collected %d bytes metadata, %d bytes counters",
		len(metaData), len(counterData))

	// Return coverage data as JSON
	response := CoverageResponse{
		MetaFilename:     metaFilename,
		CountersFilename: counterFilename,
		CountersData:     base64.StdEncoding.EncodeToString(counterData),
		Timestamp:        timestamp,
	}
	if !skipMeta {
		response.MetaData = base64.StdEncoding.EncodeToString(metaData)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[COVERAGE] Error encoding response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	log.Println("[COVERAGE] Coverage data sent successfully")
}
