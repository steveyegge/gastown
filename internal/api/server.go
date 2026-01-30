// ABOUTME: HTTP API server for Gas Town.
// ABOUTME: Exposes Gas Town functionality via REST API for web integrations.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	gtBinary string // Path to gt binary
}

// NewServer creates a new API server.
func NewServer(townRoot string, port int) *Server {
	// Find gt binary - use absolute path for reliability
	gtBinary := "gt" // Default to PATH

	// First check current directory
	if cwd, err := os.Getwd(); err == nil {
		localBin := filepath.Join(cwd, "gt")
		if _, err := os.Stat(localBin); err == nil {
			gtBinary = localBin
		}
	}

	// Check if we have the binary from os.Executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if exeDir != "" && strings.HasSuffix(exe, "gt") {
			gtBinary = exe
		}
	}

	return &Server{
		townRoot: townRoot,
		port:     port,
		gtBinary: gtBinary,
	}
}

// runGT executes a gt command and returns stdout, stderr, and error.
func (s *Server) runGT(args ...string) (string, string, error) {
	cmd := exec.Command(s.gtBinary, args...)
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runBD executes a bd command and returns stdout, stderr, and error.
func (s *Server) runBD(rigName string, args ...string) (string, string, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = filepath.Join(s.townRoot, rigName)
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
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
	mux.HandleFunc("PUT /api/rigs/{rig}/jobs/{id}", s.handleUpdateJob)
	mux.HandleFunc("POST /api/rigs/{rig}/jobs/{id}/close", s.handleCloseJob)

	// Sling (dispatch work)
	mux.HandleFunc("POST /api/rigs/{rig}/sling", s.handleSling)

	// Merge Queue
	mux.HandleFunc("GET /api/rigs/{rig}/mq", s.handleListMergeQueue)
	mux.HandleFunc("POST /api/rigs/{rig}/mq/submit", s.handleMQSubmit)

	// Polecats
	mux.HandleFunc("GET /api/rigs/{rig}/polecats", s.handleListPolecats)
	mux.HandleFunc("GET /api/rigs/{rig}/polecats/{name}", s.handleGetPolecat)
	mux.HandleFunc("POST /api/rigs/{rig}/polecats/{name}/nuke", s.handleNukePolecat)

	// Refinery
	mux.HandleFunc("GET /api/rigs/{rig}/refinery", s.handleRefineryStatus)
	mux.HandleFunc("POST /api/rigs/{rig}/refinery/start", s.handleRefineryStart)
	mux.HandleFunc("POST /api/rigs/{rig}/refinery/stop", s.handleRefineryStop)

	// Mayor
	mux.HandleFunc("GET /api/mayor", s.handleMayorStatus)
	mux.HandleFunc("POST /api/mayor/start", s.handleMayorStart)
	mux.HandleFunc("POST /api/mayor/stop", s.handleMayorStop)

	// Witness
	mux.HandleFunc("GET /api/rigs/{rig}/witness", s.handleWitnessStatus)
	mux.HandleFunc("POST /api/rigs/{rig}/witness/start", s.handleWitnessStart)
	mux.HandleFunc("POST /api/rigs/{rig}/witness/stop", s.handleWitnessStop)

	// Convoy
	mux.HandleFunc("GET /api/convoys", s.handleListConvoys)
	mux.HandleFunc("POST /api/convoys", s.handleCreateConvoy)
	mux.HandleFunc("GET /api/convoys/{id}", s.handleGetConvoy)

	// Mail
	mux.HandleFunc("POST /api/mail/send", s.handleSendMail)
	mux.HandleFunc("GET /api/rigs/{rig}/agents/{agent}/inbox", s.handleGetInbox)

	// CORS middleware
	handler := corsMiddleware(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Longer for sling operations
	}

	fmt.Printf("🚀 Gas Town API server starting on port %d\n", s.port)
	fmt.Printf("   Town root: %s\n", s.townRoot)
	fmt.Printf("   GT binary: %s\n", s.gtBinary)
	fmt.Printf("\n   Endpoints:\n")
	fmt.Printf("   Health:\n")
	fmt.Printf("     GET  /health\n")
	fmt.Printf("   Rigs:\n")
	fmt.Printf("     GET  /api/rigs\n")
	fmt.Printf("     GET  /api/rigs/{rig}\n")
	fmt.Printf("   Jobs:\n")
	fmt.Printf("     POST /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs/{id}\n")
	fmt.Printf("     PUT  /api/rigs/{rig}/jobs/{id}\n")
	fmt.Printf("     POST /api/rigs/{rig}/jobs/{id}/close\n")
	fmt.Printf("   Sling:\n")
	fmt.Printf("     POST /api/rigs/{rig}/sling\n")
	fmt.Printf("   Merge Queue:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/mq\n")
	fmt.Printf("     POST /api/rigs/{rig}/mq/submit\n")
	fmt.Printf("   Polecats:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/polecats\n")
	fmt.Printf("     GET  /api/rigs/{rig}/polecats/{name}\n")
	fmt.Printf("     POST /api/rigs/{rig}/polecats/{name}/nuke\n")
	fmt.Printf("   Refinery:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/refinery\n")
	fmt.Printf("     POST /api/rigs/{rig}/refinery/start\n")
	fmt.Printf("     POST /api/rigs/{rig}/refinery/stop\n")
	fmt.Printf("   Mayor:\n")
	fmt.Printf("     GET  /api/mayor\n")
	fmt.Printf("     POST /api/mayor/start\n")
	fmt.Printf("     POST /api/mayor/stop\n")
	fmt.Printf("   Witness:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/witness\n")
	fmt.Printf("     POST /api/rigs/{rig}/witness/start\n")
	fmt.Printf("     POST /api/rigs/{rig}/witness/stop\n")
	fmt.Printf("   Convoy:\n")
	fmt.Printf("     GET  /api/convoys\n")
	fmt.Printf("     POST /api/convoys\n")
	fmt.Printf("     GET  /api/convoys/{id}\n")
	fmt.Printf("   Mail:\n")
	fmt.Printf("     POST /api/mail/send\n")
	fmt.Printf("     GET  /api/rigs/{rig}/agents/{agent}/inbox\n")

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

// ============================================================================
// Health
// ============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"town_root": s.townRoot,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ============================================================================
// Rigs
// ============================================================================

type RigInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	HasBeads  bool   `json:"has_beads"`
	HasConfig bool   `json:"has_config"`
}

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

func (s *Server) handleGetRig(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	rigPath := filepath.Join(s.townRoot, rigName)

	configPath := filepath.Join(rigPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig not found: %s", rigName))
		return
	}

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

// ============================================================================
// Jobs (Beads)
// ============================================================================

type CreateJobRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Priority    int    `json:"priority"`
}

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

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
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

type UpdateJobRequest struct {
	Status      string `json:"status,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Description string `json:"description,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
}

func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	var req UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Build bd update command
	args := []string{"update", jobID}
	if req.Status != "" {
		args = append(args, "--status", req.Status)
	}
	if req.Notes != "" {
		args = append(args, "--notes", req.Notes)
	}
	if req.Assignee != "" {
		args = append(args, "--assignee", req.Assignee)
	}

	stdout, stderr, err := s.runBD(rigName, args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("bd update failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "updated",
		"job_id":  jobID,
		"output":  stdout,
	})
}

type CloseJobRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (s *Server) handleCloseJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	var req CloseJobRequest
	json.NewDecoder(r.Body).Decode(&req) // Optional body

	args := []string{"close", jobID}
	if req.Reason != "" {
		args = append(args, "--reason", req.Reason)
	}

	stdout, stderr, err := s.runBD(rigName, args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("bd close failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "closed",
		"job_id": jobID,
		"output": stdout,
	})
}

// ============================================================================
// Sling
// ============================================================================

type SlingRequest struct {
	BeadID   string `json:"bead_id"`
	Formula  string `json:"formula,omitempty"`
	Args     string `json:"args,omitempty"`
	NoConvoy bool   `json:"no_convoy,omitempty"`
}

func (s *Server) handleSling(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req SlingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.BeadID == "" && req.Formula == "" {
		jsonError(w, http.StatusBadRequest, "bead_id or formula is required")
		return
	}

	// Build gt sling command
	args := []string{"sling"}
	if req.Formula != "" {
		args = append(args, req.Formula)
		if req.BeadID != "" {
			args = append(args, "--on", req.BeadID)
		}
	} else {
		args = append(args, req.BeadID)
	}
	args = append(args, rigName)

	if req.Args != "" {
		args = append(args, "--args", req.Args)
	}
	if req.NoConvoy {
		args = append(args, "--no-convoy")
	}

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt sling failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":  "slung",
		"bead_id": req.BeadID,
		"rig":     rigName,
		"output":  stdout,
	})
}

// ============================================================================
// Merge Queue
// ============================================================================

func (s *Server) handleListMergeQueue(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("mq", "list", rigName, "--json")
	if err != nil {
		// Try without --json if it fails
		stdout, stderr, err = s.runGT("mq", "list", rigName)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mq list failed: %s %s", stderr, err))
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, result)
}

type MQSubmitRequest struct {
	Branch   string `json:"branch,omitempty"`
	Issue    string `json:"issue,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

func (s *Server) handleMQSubmit(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req MQSubmitRequest
	json.NewDecoder(r.Body).Decode(&req)

	args := []string{"mq", "submit"}
	if req.Branch != "" {
		args = append(args, "--branch", req.Branch)
	}
	if req.Issue != "" {
		args = append(args, "--issue", req.Issue)
	}

	// Run from rig directory
	cmd := exec.Command(s.gtBinary, args...)
	cmd.Dir = filepath.Join(s.townRoot, rigName, "refinery", "rig")
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mq submit failed: %s %s", stderr.String(), err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "submitted",
		"output": stdout.String(),
	})
}

// ============================================================================
// Polecats
// ============================================================================

func (s *Server) handleListPolecats(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("polecat", "list", rigName, "--json")
	if err != nil {
		// Fallback to non-JSON
		stdout, _, _ = s.runGT("polecat", "list", rigName)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
			"error":      stderr,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"polecats": result,
	})
}

func (s *Server) handleGetPolecat(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	name := r.PathValue("name")

	fullName := fmt.Sprintf("%s/%s", rigName, name)
	stdout, stderr, err := s.runGT("polecat", "status", fullName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt polecat status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"name":   name,
		"rig":    rigName,
		"status": stdout,
	})
}

func (s *Server) handleNukePolecat(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	name := r.PathValue("name")

	fullName := fmt.Sprintf("%s/%s", rigName, name)
	stdout, stderr, err := s.runGT("polecat", "nuke", fullName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt polecat nuke failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "nuked",
		"name":   name,
		"rig":    rigName,
		"output": stdout,
	})
}

// ============================================================================
// Refinery
// ============================================================================

func (s *Server) handleRefineryStatus(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "status", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"rig":    rigName,
		"status": stdout,
	})
}

func (s *Server) handleRefineryStart(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "start", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "started",
		"rig":    rigName,
		"output": stdout,
	})
}

func (s *Server) handleRefineryStop(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "stop", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "stopped",
		"rig":    rigName,
		"output": stdout,
	})
}

// ============================================================================
// Mayor
// ============================================================================

func (s *Server) handleMayorStatus(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "status")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": stdout,
	})
}

func (s *Server) handleMayorStart(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "start")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "started",
		"output": stdout,
	})
}

func (s *Server) handleMayorStop(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "stop")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "stopped",
		"output": stdout,
	})
}

// ============================================================================
// Witness
// ============================================================================

func (s *Server) handleWitnessStatus(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "status", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"rig":    rigName,
		"status": stdout,
	})
}

func (s *Server) handleWitnessStart(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "start", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "started",
		"rig":    rigName,
		"output": stdout,
	})
}

func (s *Server) handleWitnessStop(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "stop", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "stopped",
		"rig":    rigName,
		"output": stdout,
	})
}

// ============================================================================
// Convoy
// ============================================================================

func (s *Server) handleListConvoys(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("convoy", "list", "--json")
	if err != nil {
		stdout, _, _ = s.runGT("convoy", "list")
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
			"error":      stderr,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"convoys": result,
	})
}

type CreateConvoyRequest struct {
	Title  string   `json:"title"`
	Tracks []string `json:"tracks"` // Bead IDs to track
}

func (s *Server) handleCreateConvoy(w http.ResponseWriter, r *http.Request) {
	var req CreateConvoyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Title == "" || len(req.Tracks) == 0 {
		jsonError(w, http.StatusBadRequest, "title and tracks are required")
		return
	}

	args := []string{"convoy", "create", "--title", req.Title}
	args = append(args, req.Tracks...)

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt convoy create failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"status": "created",
		"output": stdout,
	})
}

func (s *Server) handleGetConvoy(w http.ResponseWriter, r *http.Request) {
	convoyID := r.PathValue("id")

	stdout, stderr, err := s.runGT("convoy", "status", convoyID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt convoy status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"convoy_id": convoyID,
		"status":    stdout,
	})
}

// ============================================================================
// Mail
// ============================================================================

type SendMailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *Server) handleSendMail(w http.ResponseWriter, r *http.Request) {
	var req SendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.To == "" || req.Subject == "" {
		jsonError(w, http.StatusBadRequest, "to and subject are required")
		return
	}

	args := []string{"mail", "send", req.To, "-s", req.Subject}
	if req.Body != "" {
		args = append(args, "-m", req.Body)
	}

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mail send failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "sent",
		"to":     req.To,
		"output": stdout,
	})
}

func (s *Server) handleGetInbox(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	agent := r.PathValue("agent")

	// Construct agent address
	agentAddr := fmt.Sprintf("%s/%s", rigName, agent)

	stdout, stderr, err := s.runGT("mail", "inbox", agentAddr)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mail inbox failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"agent": agentAddr,
		"inbox": stdout,
	})
}
