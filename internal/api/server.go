// ABOUTME: HTTP API server for Gas Town.
// ABOUTME: Exposes Gas Town functionality via REST API for web integrations.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// Server is the HTTP API server for Gas Town.
type Server struct {
	townRoot string
	port     int
	server   *http.Server
}

// NewServer creates a new API server.
func NewServer(townRoot string, port int) *Server {
	return &Server{
		townRoot: townRoot,
		port:     port,
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Rigs
	mux.HandleFunc("GET /api/rigs", s.handleListRigs)
	mux.HandleFunc("GET /api/rigs/{rig}", s.handleGetRig)

	// Jobs (Beads)
	mux.HandleFunc("POST /api/rigs/{rig}/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/rigs/{rig}/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/rigs/{rig}/jobs/{id}", s.handleGetJob)

	// Sling (dispatch work)
	mux.HandleFunc("POST /api/rigs/{rig}/sling", s.handleSling)

	// Merge Queue
	mux.HandleFunc("GET /api/rigs/{rig}/mq", s.handleListMergeQueue)
	mux.HandleFunc("POST /api/rigs/{rig}/mq/submit", s.handleMQSubmit)

	// Polecats
	mux.HandleFunc("GET /api/rigs/{rig}/polecats", s.handleListPolecats)

	// Refinery
	mux.HandleFunc("GET /api/rigs/{rig}/refinery", s.handleRefineryStatus)

	// CORS middleware
	handler := corsMiddleware(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	fmt.Printf("🚀 Gas Town API server starting on port %d\n", s.port)
	fmt.Printf("   Town root: %s\n", s.townRoot)
	fmt.Printf("   Endpoints:\n")
	fmt.Printf("     GET  /health\n")
	fmt.Printf("     GET  /api/rigs\n")
	fmt.Printf("     GET  /api/rigs/{rig}\n")
	fmt.Printf("     POST /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs/{id}\n")
	fmt.Printf("     POST /api/rigs/{rig}/sling\n")
	fmt.Printf("     GET  /api/rigs/{rig}/mq\n")
	fmt.Printf("     GET  /api/rigs/{rig}/polecats\n")
	fmt.Printf("     GET  /api/rigs/{rig}/refinery\n")

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// JSON response helpers
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// Health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"town_root": s.townRoot,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// RigInfo represents basic rig information.
type RigInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	HasBeads  bool   `json:"has_beads"`
	HasConfig bool   `json:"has_config"`
}

// List all rigs by scanning the town directory
func (s *Server) handleListRigs(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.townRoot)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rigs []RigInfo
	skipDirs := map[string]bool{
		"mayor": true, "deacon": true, ".beads": true,
		".claude": true, ".git": true, "plugins": true,
		"logs": true, "settings": true, "daemon": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() || skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigPath := filepath.Join(s.townRoot, entry.Name())
		configPath := filepath.Join(rigPath, "config.json")
		beadsPath := filepath.Join(rigPath, ".beads")

		_, hasConfig := os.Stat(configPath)
		_, hasBeads := os.Stat(beadsPath)

		// Only include if it has a config.json (it's a rig)
		if hasConfig == nil {
			rigs = append(rigs, RigInfo{
				Name:      entry.Name(),
				Path:      rigPath,
				HasConfig: hasConfig == nil,
				HasBeads:  hasBeads == nil,
			})
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"rigs":  rigs,
		"count": len(rigs),
	})
}

// Get rig details
func (s *Server) handleGetRig(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	rigPath := filepath.Join(s.townRoot, rigName)

	// Check if rig exists
	configPath := filepath.Join(rigPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig not found: %s", rigName))
		return
	}

	// Read config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count polecats
	polecatsPath := filepath.Join(rigPath, "polecats")
	polecatCount := 0
	if entries, err := os.ReadDir(polecatsPath); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				polecatCount++
			}
		}
	}

	// Count crew
	crewPath := filepath.Join(rigPath, "crew")
	crewCount := 0
	if entries, err := os.ReadDir(crewPath); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				crewCount++
			}
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"name":          rigName,
		"path":          rigPath,
		"config":        config,
		"polecat_count": polecatCount,
		"crew_count":    crewCount,
	})
}

// CreateJobRequest is the request body for creating a job.
type CreateJobRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Priority    int    `json:"priority"`
}

// Create a new job (bead)
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Title == "" {
		jsonError(w, http.StatusBadRequest, "title is required")
		return
	}

	// Get beads instance for this rig
	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	issueType := req.Type
	if issueType == "" {
		issueType = "task"
	}

	opts := beads.CreateOptions{
		Title:       req.Title,
		Description: req.Description,
		Type:        issueType,
		Priority:    req.Priority,
	}

	issue, err := b.Create(opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, issue)
}

// List jobs for a rig
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	// Parse query params
	status := r.URL.Query().Get("status")
	issueType := r.URL.Query().Get("type")

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	opts := beads.ListOptions{
		Status: status,
		Type:   issueType,
	}

	issues, err := b.List(opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"jobs":  issues,
		"count": len(issues),
	})
}

// Get a specific job
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	issue, err := b.Show(jobID)
	if err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("job not found: %s", jobID))
		return
	}

	jsonResponse(w, http.StatusOK, issue)
}

// SlingRequest is the request body for slinging work.
type SlingRequest struct {
	BeadID  string `json:"bead_id"`
	Formula string `json:"formula"`
}

// Sling work to a polecat
func (s *Server) handleSling(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req SlingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.BeadID == "" {
		jsonError(w, http.StatusBadRequest, "bead_id is required")
		return
	}

	// TODO: Implement sling via internal packages
	// For now, return a placeholder indicating the request was received
	jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"status":  "accepted",
		"bead_id": req.BeadID,
		"rig":     rigName,
		"message": "Sling request accepted. Use CLI for full functionality: gt sling " + req.BeadID + " " + rigName,
	})
}

// List merge queue
func (s *Server) handleListMergeQueue(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	// List merge-request type beads
	opts := beads.ListOptions{
		Type:   "merge-request",
		Status: "open",
	}

	issues, err := b.List(opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"merge_requests": issues,
		"count":          len(issues),
	})
}

// Submit to merge queue
func (s *Server) handleMQSubmit(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	// TODO: Implement MQ submit via internal packages
	jsonResponse(w, http.StatusAccepted, map[string]interface{}{
		"status":  "accepted",
		"rig":     rigName,
		"message": "MQ submit request received. Use CLI for full functionality: gt mq submit",
	})
}

// List polecats
func (s *Server) handleListPolecats(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	polecatsPath := filepath.Join(s.townRoot, rigName, "polecats")
	if _, err := os.Stat(polecatsPath); os.IsNotExist(err) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"polecats": []interface{}{},
			"count":    0,
		})
		return
	}

	entries, err := os.ReadDir(polecatsPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type PolecatInfo struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		HasClone bool   `json:"has_clone"`
	}

	var polecats []PolecatInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pcPath := filepath.Join(polecatsPath, entry.Name())
		// Check if it has a clone/rig subdirectory
		hasClone := false
		subEntries, _ := os.ReadDir(pcPath)
		for _, sub := range subEntries {
			if sub.IsDir() && sub.Name() != ".claude" {
				hasClone = true
				break
			}
		}

		polecats = append(polecats, PolecatInfo{
			Name:     entry.Name(),
			Path:     pcPath,
			HasClone: hasClone,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"polecats": polecats,
		"count":    len(polecats),
	})
}

// Refinery status
func (s *Server) handleRefineryStatus(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	refineryPath := filepath.Join(s.townRoot, rigName, "refinery")
	exists := true
	if _, err := os.Stat(refineryPath); os.IsNotExist(err) {
		exists = false
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"rig":     rigName,
		"exists":  exists,
		"path":    refineryPath,
		"message": "Use CLI for full status: gt refinery status " + rigName,
	})
}
