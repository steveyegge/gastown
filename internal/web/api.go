package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)


// CommandRequest is the JSON request body for /api/run.
type CommandRequest struct {
	// Command is the gt command to run (without the "gt" prefix).
	// Example: "status --json" or "mail inbox"
	Command string `json:"command"`
	// Timeout in seconds (optional; see WebTimeoutsConfig for defaults)
	Timeout int `json:"timeout,omitempty"`
}

// CommandResponse is the JSON response from /api/run.
type CommandResponse struct {
	Success    bool   `json:"success"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	Command    string `json:"command"`
}

// CommandListResponse is the JSON response from /api/commands.
type CommandListResponse struct {
	Commands []CommandInfo `json:"commands"`
}

// APIHandler handles API requests for the dashboard.
type APIHandler struct {
	// gtPath is the path to the gt binary. If empty, uses "gt" from PATH.
	gtPath string
	// workDir is the working directory for command execution.
	workDir string
	// Configurable timeouts (from TownSettings.WebTimeouts)
	defaultRunTimeout time.Duration
	maxRunTimeout     time.Duration
	// Options cache
	optionsCache     *OptionsResponse
	optionsCacheTime time.Time
	optionsCacheMu   sync.RWMutex
	// cmdSem limits concurrent command executions to prevent resource exhaustion.
	cmdSem chan struct{}
}

const optionsCacheTTL = 30 * time.Second

// maxConcurrentCommands limits how many gt subprocesses can run at once.
// handleOptions alone spawns 7; allow headroom for other concurrent handlers.
const maxConcurrentCommands = 12

// NewAPIHandler creates a new API handler with the given run timeouts.
func NewAPIHandler(defaultRunTimeout, maxRunTimeout time.Duration) *APIHandler {
	// Use PATH lookup for gt binary. Do NOT use os.Executable() here - during
	// tests it returns the test binary, causing fork bombs when executed.
	workDir, _ := os.Getwd()
	return &APIHandler{
		gtPath:            "gt",
		workDir:           workDir,
		defaultRunTimeout: defaultRunTimeout,
		maxRunTimeout:     maxRunTimeout,
		cmdSem:            make(chan struct{}, maxConcurrentCommands),
	}
}

// ServeHTTP routes API requests to the appropriate handler.
func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case path == "/run" && r.Method == http.MethodPost:
		h.handleRun(w, r)
	case path == "/commands" && r.Method == http.MethodGet:
		h.handleCommands(w, r)
	case path == "/options" && r.Method == http.MethodGet:
		h.handleOptions(w, r)
	case path == "/mail/inbox" && r.Method == http.MethodGet:
		h.handleMailInbox(w, r)
	case path == "/mail/read" && r.Method == http.MethodGet:
		h.handleMailRead(w, r)
	case path == "/mail/send" && r.Method == http.MethodPost:
		h.handleMailSend(w, r)
	case path == "/issues/show" && r.Method == http.MethodGet:
		h.handleIssueShow(w, r)
	case path == "/issues/create" && r.Method == http.MethodPost:
		h.handleIssueCreate(w, r)
	case path == "/issues/close" && r.Method == http.MethodPost:
		h.handleIssueClose(w, r)
	case path == "/issues/update" && r.Method == http.MethodPost:
		h.handleIssueUpdate(w, r)
	case path == "/pr/show" && r.Method == http.MethodGet:
		h.handlePRShow(w, r)
	case path == "/crew" && r.Method == http.MethodGet:
		h.handleCrew(w, r)
	case path == "/ready" && r.Method == http.MethodGet:
		h.handleReady(w, r)
	case path == "/events" && r.Method == http.MethodGet:
		h.handleSSE(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// handleRun executes a gt command and returns the result.
func (h *APIHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate command against whitelist
	meta, err := ValidateCommand(req.Command)
	if err != nil {
		h.sendError(w, fmt.Sprintf("Command blocked: %v", err), http.StatusForbidden)
		return
	}

	// Determine timeout
	timeout := h.defaultRunTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
		if timeout > h.maxRunTimeout {
			timeout = h.maxRunTimeout
		}
	}

	// Parse command into args
	args := parseCommandArgs(req.Command)
	if len(args) == 0 {
		h.sendError(w, "Empty command", http.StatusBadRequest)
		return
	}

	// Sanitize args
	args = SanitizeArgs(args)

	// Execute command
	start := time.Now()
	output, err := h.runGtCommand(r.Context(), timeout, args)
	duration := time.Since(start)

	resp := CommandResponse{
		Command:    req.Command,
		DurationMs: duration.Milliseconds(),
	}

	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		resp.Output = output // Include partial output on error
	} else {
		resp.Success = true
		resp.Output = output
	}

	// Log command execution (but not for safe read-only commands to reduce noise)
	if !meta.Safe || !resp.Success {
		// Could add structured logging here
		_ = meta // silence unused warning for now
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleCommands returns the list of available commands for the palette.
func (h *APIHandler) handleCommands(w http.ResponseWriter, _ *http.Request) {
	resp := CommandListResponse{
		Commands: GetCommandList(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// runGtCommand executes a gt command with the given args.
func (h *APIHandler) runGtCommand(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	return runCommand(ctx, timeout, h.gtPath, args, h.workDir, h.cmdSem)
}

// runBdCommand executes a bd command with the given args.
func (h *APIHandler) runBdCommand(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	return runCommand(ctx, timeout, "bd", args, h.workDir, h.cmdSem)
}

// runGhCommand executes a gh command with the given args.
func (h *APIHandler) runGhCommand(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	return runCommand(ctx, timeout, "gh", args, h.workDir, h.cmdSem)
}

// sendError sends a JSON error response.
func (h *APIHandler) sendError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(CommandResponse{
		Success: false,
		Error:   message,
	})
}

// OptionItem represents an option with name and status.
type OptionItem struct {
	Name    string `json:"name"`
	Status  string `json:"status,omitempty"`  // "running", "stopped", "idle", etc.
	Running bool   `json:"running,omitempty"` // convenience field
}

// OptionsResponse is the JSON response from /api/options.
type OptionsResponse struct {
	Rigs        []string     `json:"rigs,omitempty"`
	Polecats    []string     `json:"polecats,omitempty"`
	Convoys     []string     `json:"convoys,omitempty"`
	Agents      []OptionItem `json:"agents,omitempty"`
	Hooks       []string     `json:"hooks,omitempty"`
	Messages    []string     `json:"messages,omitempty"`
	Crew        []string     `json:"crew,omitempty"`
	Escalations []string     `json:"escalations,omitempty"`
}

// handleOptions returns dynamic options for command arguments.
// Results are cached for 30 seconds to avoid slow repeated fetches.
func (h *APIHandler) handleOptions(w http.ResponseWriter, r *http.Request) {
	// Check cache first — serialize under RLock to a buffer so we don't
	// hold the lock while writing to the ResponseWriter (which can block
	// on slow clients).
	h.optionsCacheMu.RLock()
	if h.optionsCache != nil && time.Since(h.optionsCacheTime) < optionsCacheTTL {
		data, err := json.Marshal(h.optionsCache)
		h.optionsCacheMu.RUnlock()
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n"))
			return
		}
		// Marshal failure is unexpected; fall through to refetch.
	} else {
		h.optionsCacheMu.RUnlock()
	}

	// Cache miss - fetch fresh data
	resp := &OptionsResponse{}
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Run all fetches in parallel with shorter timeouts
	wg.Add(7)

	// Fetch rigs
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"rig", "list"}); err == nil {
			mu.Lock()
			resp.Rigs = parseRigListOutput(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: rig list: %v", err)
		}
	}()

	// Fetch polecats
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"polecat", "list", "--all", "--json"}); err == nil {
			mu.Lock()
			resp.Polecats = parseJSONPaths(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: polecat list: %v", err)
		}
	}()

	// Fetch convoys
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"convoy", "list"}); err == nil {
			mu.Lock()
			resp.Convoys = parseConvoyListOutput(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: convoy list: %v", err)
		}
	}()

	// Fetch hooks
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"hooks", "list"}); err == nil {
			mu.Lock()
			resp.Hooks = parseHooksListOutput(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: hooks list: %v", err)
		}
	}()

	// Fetch mail messages
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"mail", "inbox"}); err == nil {
			mu.Lock()
			resp.Messages = parseMailInboxOutput(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: mail inbox: %v", err)
		}
	}()

	// Fetch crew members
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 3*time.Second, []string{"crew", "list", "--all"}); err == nil {
			mu.Lock()
			resp.Crew = parseCrewListOutput(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: crew list: %v", err)
		}
	}()

	// Fetch agents - shorter timeout, skip if slow
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"status", "--json"}); err == nil {
			mu.Lock()
			resp.Agents = parseAgentsFromStatus(output)
			mu.Unlock()
		} else {
			log.Printf("warning: handleOptions: status: %v", err)
		}
	}()

	wg.Wait()

	// Update cache
	h.optionsCacheMu.Lock()
	h.optionsCache = resp
	h.optionsCacheTime = time.Now()
	h.optionsCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	_ = json.NewEncoder(w).Encode(resp)
}

// parseRigListOutput extracts rig names from the text output of "gt rig list".
// Example output:
//
//	Rigs in /Users/foo/gt:
//	  claycantrell
//	    Polecats: 1  Crew: 2
//	  gastown
//	    Polecats: 1  Crew: 1
func parseRigListOutput(output string) []string {
	var rigs []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Rig names are indented with 2 spaces and no colon
		trimmed := strings.TrimPrefix(line, "  ")
		if trimmed != line && !strings.Contains(trimmed, ":") && strings.TrimSpace(trimmed) != "" {
			// This is a rig name line
			name := strings.TrimSpace(trimmed)
			if name != "" && !strings.HasPrefix(name, "Rigs") {
				rigs = append(rigs, name)
			}
		}
	}
	return rigs
}

// parseConvoyListOutput extracts convoy IDs from text output.
func parseConvoyListOutput(output string) []string {
	var convoys []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Look for lines that start with convoy ID pattern
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "Convoy") && !strings.HasPrefix(trimmed, "No ") {
			// Try to extract the first word as convoy ID
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				convoys = append(convoys, parts[0])
			}
		}
	}
	return convoys
}

// parseHooksListOutput extracts bead names from hooks list output.
func parseHooksListOutput(output string) []string {
	var hooks []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip header lines and empty lines
		if trimmed != "" && !strings.HasPrefix(trimmed, "Hook") && !strings.HasPrefix(trimmed, "No ") && !strings.HasPrefix(trimmed, "BEAD") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				hooks = append(hooks, parts[0])
			}
		}
	}
	return hooks
}

// parseMailInboxOutput extracts message IDs from mail inbox output.
func parseMailInboxOutput(output string) []string {
	var messages []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip header lines and empty lines
		if trimmed != "" && !strings.HasPrefix(trimmed, "Mail") && !strings.HasPrefix(trimmed, "No ") && !strings.HasPrefix(trimmed, "ID") && !strings.HasPrefix(trimmed, "---") {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				messages = append(messages, parts[0])
			}
		}
	}
	return messages
}

// parseCrewListOutput extracts crew member names (rig/name format) from crew list output.
func parseCrewListOutput(output string) []string {
	var crew []string
	lines := strings.Split(output, "\n")
	currentRig := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Check if this is a rig header (ends with :)
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			currentRig = strings.TrimSuffix(trimmed, ":")
			continue
		}
		// Skip non-crew lines
		if strings.HasPrefix(trimmed, "Crew") || strings.HasPrefix(trimmed, "No ") {
			continue
		}
		// This should be a crew member name
		if currentRig != "" {
			parts := strings.Fields(trimmed)
			if len(parts) > 0 {
				crew = append(crew, currentRig+"/"+parts[0])
			}
		}
	}
	return crew
}

// parseAgentsFromStatus extracts agents with status from "gt status --json" output.
func parseAgentsFromStatus(jsonStr string) []OptionItem {
	var status struct {
		Agents []struct {
			Name    string `json:"name"`
			Running bool   `json:"running"`
			State   string `json:"state"`
		} `json:"agents"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &status); err != nil {
		return nil
	}

	var agents []OptionItem
	for _, a := range status.Agents {
		state := a.State
		if state == "" {
			if a.Running {
				state = "running"
			} else {
				state = "stopped"
			}
		}
		agents = append(agents, OptionItem{
			Name:    a.Name,
			Status:  state,
			Running: a.Running,
		})
	}
	return agents
}

// parseJSONPaths extracts rig/name paths from polecat JSON output.
func parseJSONPaths(jsonStr string) []string {
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		var wrapper map[string][]map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
			return nil
		}
		for _, v := range wrapper {
			items = v
			break
		}
	}

	var paths []string
	for _, item := range items {
		rig, _ := item["rig"].(string)
		name, _ := item["name"].(string)
		if rig != "" && name != "" {
			paths = append(paths, rig+"/"+name)
		}
	}
	return paths
}

// parseCommandArgs splits a command string into args, respecting quotes.
func parseCommandArgs(command string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range command {
		switch {
		case r == '"' || r == '\'':
			if inQuote && r == quoteChar {
				inQuote = false
				quoteChar = 0
			} else if !inQuote {
				inQuote = true
				quoteChar = r
			} else {
				current.WriteRune(r)
			}
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}
