package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCommandTimeout is the default timeout for command execution.
	DefaultCommandTimeout = 30 * time.Second
	// MaxCommandTimeout is the maximum allowed timeout.
	MaxCommandTimeout = 60 * time.Second
)

// CommandRequest is the JSON request body for /api/run.
type CommandRequest struct {
	// Command is the gt command to run (without the "gt" prefix).
	// Example: "status --json" or "mail inbox"
	Command string `json:"command"`
	// Timeout in seconds (optional, default 30, max 60)
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
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler() *APIHandler {
	// Use the current executable as the gt binary (we ARE gt)
	gtPath := "gt"
	if exe, err := os.Executable(); err == nil {
		gtPath = exe
	}
	// Capture the current working directory for command execution
	workDir, _ := os.Getwd()
	return &APIHandler{
		gtPath:  gtPath,
		workDir: workDir,
	}
}

// ServeHTTP routes API requests to the appropriate handler.
func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for dashboard
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

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
	case path == "/pr/show" && r.Method == http.MethodGet:
		h.handlePRShow(w, r)
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
	timeout := DefaultCommandTimeout
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
		if timeout > MaxCommandTimeout {
			timeout = MaxCommandTimeout
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
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.gtPath, args...)
	if h.workDir != "" {
		cmd.Dir = h.workDir
	}
	// Ensure the command doesn't wait for stdin
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out after %v", timeout)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %v", err)
	}

	return output, nil
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

// MailMessage represents a mail message for the API.
type MailMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body,omitempty"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Priority  string `json:"priority,omitempty"`
}

// MailInboxResponse is the response for /api/mail/inbox.
type MailInboxResponse struct {
	Messages    []MailMessage `json:"messages"`
	UnreadCount int           `json:"unread_count"`
	Total       int           `json:"total"`
}

// handleMailInbox returns the user's inbox.
func (h *APIHandler) handleMailInbox(w http.ResponseWriter, r *http.Request) {
	output, err := h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "inbox", "--json"})
	if err != nil {
		// Try without --json flag
		output, err = h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "inbox"})
		if err != nil {
			h.sendError(w, "Failed to fetch inbox: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Parse text output
		messages := parseMailInboxText(output)
		unread := 0
		for _, m := range messages {
			if !m.Read {
				unread++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MailInboxResponse{
			Messages:    messages,
			UnreadCount: unread,
			Total:       len(messages),
		})
		return
	}

	// Parse JSON output
	var messages []MailMessage
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		h.sendError(w, "Failed to parse inbox: "+err.Error(), http.StatusInternalServerError)
		return
	}

	unread := 0
	for _, m := range messages {
		if !m.Read {
			unread++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(MailInboxResponse{
		Messages:    messages,
		UnreadCount: unread,
		Total:       len(messages),
	})
}

// handleMailRead reads a specific message by ID.
func (h *APIHandler) handleMailRead(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("id")
	if msgID == "" {
		h.sendError(w, "Missing message ID", http.StatusBadRequest)
		return
	}

	output, err := h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "read", msgID})
	if err != nil {
		h.sendError(w, "Failed to read message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the message output
	msg := parseMailReadOutput(output, msgID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}

// MailSendRequest is the request body for /api/mail/send.
type MailSendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// handleMailSend sends a new message.
func (h *APIHandler) handleMailSend(w http.ResponseWriter, r *http.Request) {
	var req MailSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Subject == "" {
		h.sendError(w, "Missing required fields (to, subject)", http.StatusBadRequest)
		return
	}

	args := []string{"mail", "send", req.To, "-s", req.Subject}
	if req.Body != "" {
		args = append(args, "-m", req.Body)
	}
	if req.ReplyTo != "" {
		args = append(args, "--reply-to", req.ReplyTo)
	}

	output, err := h.runGtCommand(r.Context(), 30*time.Second, args)
	if err != nil {
		h.sendError(w, "Failed to send message: "+err.Error()+"\n"+output, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Message sent",
		"output":  output,
	})
}

// parseMailInboxText parses text output from "gt mail inbox".
func parseMailInboxText(output string) []MailMessage {
	var messages []MailMessage
	lines := strings.Split(output, "\n")

	// Format: "  1. ● subject" or "  1. subject" (● = unread)
	// followed by "      id from sender"
	// followed by "      timestamp"
	var current *MailMessage
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "📬") || strings.HasPrefix(trimmed, "(no messages)") {
			continue
		}

		// Check for numbered message line
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == '.' {
			// Save previous message
			if current != nil {
				messages = append(messages, *current)
			}
			current = &MailMessage{}
			// Parse "1. ● subject" or "1. subject"
			rest := strings.TrimSpace(trimmed[2:])
			if strings.HasPrefix(rest, "●") {
				current.Read = false
				current.Subject = strings.TrimSpace(rest[len("●"):])
			} else {
				current.Read = true
				current.Subject = rest
			}
		} else if current != nil && current.ID == "" && strings.Contains(trimmed, " from ") {
			// Parse "id from sender"
			parts := strings.SplitN(trimmed, " from ", 2)
			if len(parts) == 2 {
				current.ID = strings.TrimSpace(parts[0])
				current.From = strings.TrimSpace(parts[1])
			}
		} else if current != nil && current.Timestamp == "" && (strings.Contains(trimmed, "-") || strings.Contains(trimmed, ":")) {
			current.Timestamp = trimmed
		}
	}
	// Don't forget the last one
	if current != nil && current.ID != "" {
		messages = append(messages, *current)
	}

	return messages
}

// parseMailReadOutput parses the output from "gt mail read <id>".
func parseMailReadOutput(output string, msgID string) MailMessage {
	msg := MailMessage{ID: msgID}
	lines := strings.Split(output, "\n")

	inBody := false
	var bodyLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "📬 ") || strings.HasPrefix(line, "Subject: ") {
			msg.Subject = strings.TrimPrefix(strings.TrimPrefix(line, "📬 "), "Subject: ")
			msg.Subject = strings.TrimSpace(msg.Subject)
		} else if strings.HasPrefix(line, "From: ") {
			msg.From = strings.TrimPrefix(line, "From: ")
		} else if strings.HasPrefix(line, "To: ") {
			msg.To = strings.TrimPrefix(line, "To: ")
		} else if strings.HasPrefix(line, "ID: ") {
			msg.ID = strings.TrimPrefix(line, "ID: ")
		} else if line == "" && msg.From != "" && !inBody {
			inBody = true
		} else if inBody {
			bodyLines = append(bodyLines, line)
		}
	}

	msg.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return msg
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
// All commands are run in parallel to minimize response time.
func (h *APIHandler) handleOptions(w http.ResponseWriter, r *http.Request) {
	resp := OptionsResponse{}
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Run all fetches in parallel
	wg.Add(7)

	// Fetch rigs (parse text output - no JSON support)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"rig", "list"}); err == nil {
			mu.Lock()
			resp.Rigs = parseRigListOutput(output)
			mu.Unlock()
		}
	}()

	// Fetch polecats (has JSON support)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"polecat", "list", "--all", "--json"}); err == nil {
			mu.Lock()
			resp.Polecats = parseJSONPaths(output) // rig/name format
			mu.Unlock()
		}
	}()

	// Fetch convoys (parse text output)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"convoy", "list"}); err == nil {
			mu.Lock()
			resp.Convoys = parseConvoyListOutput(output)
			mu.Unlock()
		}
	}()

	// Fetch hooks (parse text output)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"hooks", "list"}); err == nil {
			mu.Lock()
			resp.Hooks = parseHooksListOutput(output)
			mu.Unlock()
		}
	}()

	// Fetch mail messages (parse text output)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"mail", "inbox"}); err == nil {
			mu.Lock()
			resp.Messages = parseMailInboxOutput(output)
			mu.Unlock()
		}
	}()

	// Fetch crew members (parse text output)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 5*time.Second, []string{"crew", "list", "--all"}); err == nil {
			mu.Lock()
			resp.Crew = parseCrewListOutput(output)
			mu.Unlock()
		}
	}()

	// Fetch agents with status from status --json (needs longer timeout - can take 20+ seconds)
	go func() {
		defer wg.Done()
		if output, err := h.runGtCommand(r.Context(), 30*time.Second, []string{"status", "--json"}); err == nil {
			mu.Lock()
			resp.Agents = parseAgentsFromStatus(output)
			mu.Unlock()
		}
	}()

	// Wait for all commands to complete
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
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

// parseJSONNames extracts a field from a JSON array of objects.
func parseJSONNames(jsonStr string, field string) []string {
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		// Try parsing as object with array field
		var wrapper map[string][]map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
			return nil
		}
		for _, v := range wrapper {
			items = v
			break
		}
	}

	var names []string
	for _, item := range items {
		if name, ok := item[field].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
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

// IssueShowResponse is the response for /api/issues/show.
type IssueShowResponse struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Description string   `json:"description,omitempty"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Blocks      []string `json:"blocks,omitempty"`
	RawOutput   string   `json:"raw_output"`
}

// handleIssueShow returns details for a specific issue/bead.
func (h *APIHandler) handleIssueShow(w http.ResponseWriter, r *http.Request) {
	issueID := r.URL.Query().Get("id")
	if issueID == "" {
		h.sendError(w, "Missing issue ID", http.StatusBadRequest)
		return
	}

	// Run bd show to get issue details
	output, err := h.runBdCommand(r.Context(), 10*time.Second, []string{"show", issueID})
	if err != nil {
		h.sendError(w, "Failed to fetch issue: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the output
	resp := parseIssueShowOutput(output, issueID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// runBdCommand executes a bd command with the given args.
func (h *APIHandler) runBdCommand(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", args...)
	if h.workDir != "" {
		cmd.Dir = h.workDir
	}
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out after %v", timeout)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %v", err)
	}

	return output, nil
}

// parseIssueShowOutput parses the output from "bd show <id>".
func parseIssueShowOutput(output string, issueID string) IssueShowResponse {
	resp := IssueShowResponse{
		ID:        issueID,
		RawOutput: output,
	}

	lines := strings.Split(output, "\n")
	inDescription := false
	parsedFirstLine := false
	var descLines []string
	var dependsOn []string
	var blocks []string

	for _, line := range lines {
		// First non-empty line usually has the format: "○ id · title   [● P2 · OPEN]"
		if !parsedFirstLine && (strings.HasPrefix(line, "○") || strings.HasPrefix(line, "●")) {
			parsedFirstLine = true
			// Parse the first line for title and status
			// Format: "○ id · title   [● P2 · OPEN]"
			// Find the bracket first to isolate the status
			if bracketIdx := strings.Index(line, "["); bracketIdx > 0 {
				beforeBracket := line[:bracketIdx]
				statusPart := line[bracketIdx:]

				// Extract priority and status from [● P2 · OPEN]
				statusPart = strings.Trim(statusPart, "[]●○ ")
				statusParts := strings.Split(statusPart, "·")
				if len(statusParts) >= 1 {
					resp.Priority = strings.TrimSpace(statusParts[0])
				}
				if len(statusParts) >= 2 {
					resp.Status = strings.TrimSpace(statusParts[1])
				}

				// Now parse the title from before the bracket
				// Format: "○ id · title"
				// Find the second · which separates id from title
				if firstDot := strings.Index(beforeBracket, "·"); firstDot > 0 {
					afterFirstDot := beforeBracket[firstDot+len("·"):]
					if secondDot := strings.Index(afterFirstDot, "·"); secondDot > 0 {
						resp.Title = strings.TrimSpace(afterFirstDot[secondDot+len("·"):])
					} else {
						// Only one dot - id is embedded in icon part
						resp.Title = strings.TrimSpace(afterFirstDot)
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "Type:") {
			resp.Type = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		} else if strings.HasPrefix(line, "Created:") {
			parts := strings.Split(line, "·")
			resp.Created = strings.TrimSpace(strings.TrimPrefix(parts[0], "Created:"))
			if len(parts) > 1 {
				resp.Updated = strings.TrimSpace(strings.TrimPrefix(parts[1], "Updated:"))
			}
		} else if line == "DESCRIPTION" {
			inDescription = true
		} else if line == "DEPENDS ON" || line == "BLOCKS" {
			inDescription = false
		} else if inDescription && strings.TrimSpace(line) != "" {
			descLines = append(descLines, line)
		} else if strings.HasPrefix(strings.TrimSpace(line), "→") {
			// Dependency line
			depLine := strings.TrimSpace(line)
			depLine = strings.TrimPrefix(depLine, "→")
			depLine = strings.TrimSpace(depLine)
			// Extract just the bead ID
			if colonIdx := strings.Index(depLine, ":"); colonIdx > 0 {
				parts := strings.Fields(depLine[:colonIdx])
				if len(parts) >= 2 {
					dependsOn = append(dependsOn, parts[1])
				}
			}
		} else if strings.HasPrefix(strings.TrimSpace(line), "←") {
			// Blocks line
			blockLine := strings.TrimSpace(line)
			blockLine = strings.TrimPrefix(blockLine, "←")
			blockLine = strings.TrimSpace(blockLine)
			// Extract just the bead ID
			if colonIdx := strings.Index(blockLine, ":"); colonIdx > 0 {
				parts := strings.Fields(blockLine[:colonIdx])
				if len(parts) >= 2 {
					blocks = append(blocks, parts[1])
				}
			}
		}
	}

	resp.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
	resp.DependsOn = dependsOn
	resp.Blocks = blocks

	return resp
}

// PRShowResponse is the response for /api/pr/show.
type PRShowResponse struct {
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	Author       string   `json:"author"`
	URL          string   `json:"url"`
	Body         string   `json:"body"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
	ChangedFiles int      `json:"changed_files"`
	Mergeable    string   `json:"mergeable"`
	BaseRef      string   `json:"base_ref"`
	HeadRef      string   `json:"head_ref"`
	Labels       []string `json:"labels,omitempty"`
	Checks       []string `json:"checks,omitempty"`
	RawOutput    string   `json:"raw_output,omitempty"`
}

// handlePRShow returns details for a specific PR.
func (h *APIHandler) handlePRShow(w http.ResponseWriter, r *http.Request) {
	// Accept either repo/number or full URL
	repo := r.URL.Query().Get("repo")
	number := r.URL.Query().Get("number")
	prURL := r.URL.Query().Get("url")

	if prURL == "" && (repo == "" || number == "") {
		h.sendError(w, "Missing repo/number or url parameter", http.StatusBadRequest)
		return
	}

	var args []string
	if prURL != "" {
		args = []string{"pr", "view", prURL, "--json", "number,title,state,author,url,body,createdAt,updatedAt,additions,deletions,changedFiles,mergeable,baseRefName,headRefName,labels,statusCheckRollup"}
	} else {
		args = []string{"pr", "view", number, "--repo", repo, "--json", "number,title,state,author,url,body,createdAt,updatedAt,additions,deletions,changedFiles,mergeable,baseRefName,headRefName,labels,statusCheckRollup"}
	}

	output, err := h.runGhCommand(r.Context(), 15*time.Second, args)
	if err != nil {
		h.sendError(w, "Failed to fetch PR: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the JSON output
	resp := parsePRShowOutput(output)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// runGhCommand executes a gh command with the given args.
func (h *APIHandler) runGhCommand(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	if h.workDir != "" {
		cmd.Dir = h.workDir
	}
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out after %v", timeout)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %v", err)
	}

	return output, nil
}

// parsePRShowOutput parses the JSON output from "gh pr view --json".
func parsePRShowOutput(jsonStr string) PRShowResponse {
	resp := PRShowResponse{
		RawOutput: jsonStr,
	}

	var data struct {
		Number       int    `json:"number"`
		Title        string `json:"title"`
		State        string `json:"state"`
		Author       struct {
			Login string `json:"login"`
		} `json:"author"`
		URL          string `json:"url"`
		Body         string `json:"body"`
		CreatedAt    string `json:"createdAt"`
		UpdatedAt    string `json:"updatedAt"`
		Additions    int    `json:"additions"`
		Deletions    int    `json:"deletions"`
		ChangedFiles int    `json:"changedFiles"`
		Mergeable    string `json:"mergeable"`
		BaseRefName  string `json:"baseRefName"`
		HeadRefName  string `json:"headRefName"`
		Labels       []struct {
			Name string `json:"name"`
		} `json:"labels"`
		StatusCheckRollup []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"statusCheckRollup"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return resp
	}

	resp.Number = data.Number
	resp.Title = data.Title
	resp.State = data.State
	resp.Author = data.Author.Login
	resp.URL = data.URL
	resp.Body = data.Body
	resp.CreatedAt = data.CreatedAt
	resp.UpdatedAt = data.UpdatedAt
	resp.Additions = data.Additions
	resp.Deletions = data.Deletions
	resp.ChangedFiles = data.ChangedFiles
	resp.Mergeable = data.Mergeable
	resp.BaseRef = data.BaseRefName
	resp.HeadRef = data.HeadRefName

	for _, label := range data.Labels {
		resp.Labels = append(resp.Labels, label.Name)
	}

	for _, check := range data.StatusCheckRollup {
		status := check.Name + ": "
		if check.Conclusion != "" {
			status += check.Conclusion
		} else {
			status += check.Status
		}
		resp.Checks = append(resp.Checks, status)
	}

	// Clear raw output if parsing succeeded
	resp.RawOutput = ""

	return resp
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
