package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SetupHandler handles the setup flow when no workspace exists.
type SetupHandler struct {
	html []byte
}

// NewSetupHandler creates a new setup handler, loading the setup template.
func NewSetupHandler() (*SetupHandler, error) {
	html, err := templateFS.ReadFile("templates/setup.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read setup template: %w", err)
	}
	return &SetupHandler{html: html}, nil
}

// ServeHTTP renders the setup page.
func (h *SetupHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(h.html)
}

// SetupAPIHandler handles API requests for setup operations.
type SetupAPIHandler struct{}

// NewSetupAPIHandler creates a new setup API handler.
func NewSetupAPIHandler() *SetupAPIHandler {
	return &SetupAPIHandler{}
}

// ServeHTTP routes setup API requests.
func (h *SetupAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/install" && r.Method == http.MethodPost:
		h.handleInstall(w, r)
	case path == "/rig/add" && r.Method == http.MethodPost:
		h.handleRigAdd(w, r)
	case path == "/check-workspace" && r.Method == http.MethodPost:
		h.handleCheckWorkspace(w, r)
	case path == "/launch" && r.Method == http.MethodPost:
		h.handleLaunch(w, r)
	case path == "/status" && r.Method == http.MethodGet:
		h.handleStatus(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// InstallRequest is the request body for installing a new workspace.
type InstallRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Git  bool   `json:"git"`
}

// CheckWorkspaceRequest is the request body for checking a workspace path.
type CheckWorkspaceRequest struct {
	Path string `json:"path"`
}

// LaunchRequest is the request body for launching dashboard from a workspace.
type LaunchRequest struct {
	Path string `json:"path"`
	Port int    `json:"port"`
}

// CheckWorkspaceResponse is the response for workspace checks.
type CheckWorkspaceResponse struct {
	Valid   bool     `json:"valid"`
	Path    string   `json:"path"`
	Name    string   `json:"name,omitempty"`
	Rigs    []string `json:"rigs,omitempty"`
	Message string   `json:"message,omitempty"`
}

// RigAddRequest is the request body for adding a rig.
type RigAddRequest struct {
	Name   string `json:"name"`
	GitURL string `json:"gitUrl"`
}

// SetupResponse is the response for setup operations.
type SetupResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Output  string `json:"output,omitempty"`
}

func (h *SetupAPIHandler) handleInstall(w http.ResponseWriter, r *http.Request) {
	var req InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		writeError(w, "Path is required", http.StatusBadRequest)
		return
	}

	// Expand ~ to home directory (with path cleaning to prevent traversal).
	// Absolute paths (e.g., /opt/workspace) are intentionally allowed —
	// this is a localhost-only dashboard and users may install workspaces anywhere.
	expanded, err := expandHomePath(req.Path)
	if err != nil {
		log.Printf("handleInstall: expandHomePath(%q) failed: %v", req.Path, err)
		writeError(w, "Invalid path", http.StatusBadRequest)
		return
	}
	req.Path = expanded

	// Build gt install command. Flags go first, then -- to end flag parsing,
	// then the positional path (prevents paths like "--help" being parsed as flags).
	args := []string{"install"}
	if req.Name != "" {
		if !isValidID(req.Name) {
			writeError(w, "Invalid workspace name format", http.StatusBadRequest)
			return
		}
		args = append(args, "--name", req.Name)
	}
	if req.Git {
		args = append(args, "--git")
	}
	args = append(args, "--", req.Path)

	output, err := runCommand(r.Context(), 60*time.Second, "gt", args, "", nil)
	if err != nil {
		writeJSON(w, SetupResponse{
			Success: false,
			Error:   err.Error(),
			Output:  output,
		})
		return
	}

	writeJSON(w, SetupResponse{
		Success: true,
		Message: fmt.Sprintf("Workspace created at %s", req.Path),
		Output:  output,
	})
}

func (h *SetupAPIHandler) handleRigAdd(w http.ResponseWriter, r *http.Request) {
	var req RigAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.GitURL == "" {
		writeError(w, "Name and gitUrl are required", http.StatusBadRequest)
		return
	}
	if !isValidRigName(req.Name) {
		writeError(w, "Invalid rig name format (alphanumeric and underscores only, no hyphens or dots)", http.StatusBadRequest)
		return
	}
	if !isValidGitURL(req.GitURL) {
		writeError(w, "Git URL must be https://, http://, ssh://, git://, or git@host:path format", http.StatusBadRequest)
		return
	}

	// Flags before --, positional args after (consistent with handleInstall/handleIssueCreate).
	args := []string{"rig", "add", "--", req.Name, req.GitURL}

	output, err := runCommand(r.Context(), 120*time.Second, "gt", args, "", nil)
	if err != nil {
		writeJSON(w, SetupResponse{
			Success: false,
			Error:   err.Error(),
			Output:  output,
		})
		return
	}

	writeJSON(w, SetupResponse{
		Success: true,
		Message: fmt.Sprintf("Rig '%s' added", req.Name),
		Output:  output,
	})
}

func (h *SetupAPIHandler) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req LaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		writeError(w, "Path is required", http.StatusBadRequest)
		return
	}

	// Expand ~ to home directory (with path cleaning to prevent traversal)
	path, err := expandHomePath(req.Path)
	if err != nil {
		log.Printf("handleLaunch: expandHomePath(%q) failed: %v", req.Path, err)
		writeError(w, "Invalid path", http.StatusBadRequest)
		return
	}

	port := req.Port
	if port == 0 {
		port = 8080
	}
	// Upper bound is 65534 (not 65535) to reserve room for newPort = port + 1
	if port < 1 || port > 65534 {
		writeError(w, "Port must be between 1 and 65534", http.StatusBadRequest)
		return
	}

	// Use PATH lookup for gt binary. Do NOT use os.Executable() here - during
	// tests it returns the test binary, causing fork bombs when executed.

	// Start new dashboard on a DIFFERENT port first, then we'll tell the browser to go there
	newPort := port + 1

	// Start new dashboard process from the workspace directory
	cmd := exec.Command("gt", "dashboard", "--port", fmt.Sprintf("%d", newPort))
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		writeError(w, "Failed to start dashboard: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Wait for the new server to be ready
	ready := false
	for i := 0; i < 30; i++ { // Try for 3 seconds
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/commands", newPort))
		if err == nil {
			_ = resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		writeError(w, "New dashboard failed to start", http.StatusInternalServerError)
		return
	}

	// Send success response with the new port to redirect to
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Dashboard launching from %s", path),
		"redirect": fmt.Sprintf("http://localhost:%d", newPort),
	})
}

func (h *SetupAPIHandler) handleCheckWorkspace(w http.ResponseWriter, r *http.Request) {
	var req CheckWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CheckWorkspaceResponse{Valid: false, Message: "Path is required"})
		return
	}

	// Expand ~ to home directory (with path cleaning to prevent traversal)
	path, err := expandHomePath(req.Path)
	if err != nil {
		// Return 200 with Valid:false (not 400) because this is a "check" endpoint
		// that reports validity status, unlike mutating endpoints that return 400 on bad input.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CheckWorkspaceResponse{Valid: false, Message: "Invalid path format"})
		return
	}

	// Check if mayor/ directory exists (indicates a Gas Town HQ)
	mayorDir := filepath.Join(path, "mayor")
	if _, err := os.Stat(mayorDir); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CheckWorkspaceResponse{
			Valid:   false,
			Path:    path,
			Message: "Not a Gas Town workspace (no mayor/ directory)",
		})
		return
	}

	// Try to get rig list from this workspace
	var rigs []string
	cmd := exec.CommandContext(r.Context(), "gt", "rig", "list", "--json")
	cmd.Dir = path
	if output, err := cmd.Output(); err == nil {
		// Parse JSON output for rig names
		var rigList []struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(output, &rigList) == nil {
			for _, rig := range rigList {
				rigs = append(rigs, rig.Name)
			}
		}
	}

	// Get workspace name from directory
	name := filepath.Base(path)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CheckWorkspaceResponse{
		Valid:   true,
		Path:    path,
		Name:    name,
		Rigs:    rigs,
		Message: fmt.Sprintf("Valid workspace with %d rigs", len(rigs)),
	})
}

func (h *SetupAPIHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Check if we can find a workspace now
	output, err := runCommand(r.Context(), 5*time.Second, "gt", []string{"status"}, "", nil)
	if err != nil {
		writeJSON(w, SetupResponse{
			Success: false,
			Error:   "No workspace configured",
		})
		return
	}

	writeJSON(w, SetupResponse{
		Success: true,
		Message: "Workspace found",
		Output:  output,
	})
}

// NewSetupMux creates the HTTP handler for setup mode.
func NewSetupMux() (http.Handler, error) {
	setupHandler, err := NewSetupHandler()
	if err != nil {
		return nil, err
	}
	apiHandler := NewSetupAPIHandler()

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", setupHandler)

	return corsMiddleware(mux), nil
}
