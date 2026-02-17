package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// IssueShowResponse is the response for /api/issues/show.
type IssueShowResponse struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Owner       string   `json:"owner,omitempty"`
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
	// Issue IDs may use external:prefix:id format for cross-rig dependencies
	// (see internal/web/fetcher.go:extractIssueID). Unwrap to the raw bead ID
	// before validation and before passing to bd show, which doesn't handle
	// the external: prefix. This also fixes a pre-existing bug where the
	// wrapped ID was passed to bd show and always failed to resolve.
	showID := issueID
	if strings.HasPrefix(issueID, "external:") {
		parts := strings.SplitN(issueID, ":", 3)
		if len(parts) == 3 {
			showID = parts[2]
		} else {
			h.sendError(w, "Malformed external issue ID (expected external:prefix:id)", http.StatusBadRequest)
			return
		}
	}
	if !isValidID(showID) {
		h.sendError(w, "Invalid issue ID format", http.StatusBadRequest)
		return
	}

	// Try structured JSON output first (preferred — no text parsing needed)
	output, err := h.runBdCommand(r.Context(), 10*time.Second, []string{"show", showID, "--json"})
	if err == nil {
		if resp, ok := parseIssueShowJSON(output); ok {
			// Preserve the original request ID in the response (may be external:prefix:id).
			// Callers may store/compare the full prefixed form.
			resp.ID = issueID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Fall back to text parsing
	output, err = h.runBdCommand(r.Context(), 10*time.Second, []string{"show", showID})
	if err != nil {
		h.sendError(w, "Failed to fetch issue: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Pass issueID (not showID) to preserve the original ID in the API response.
	// Callers may store/compare the full external:prefix:id form.
	resp := parseIssueShowOutput(output, issueID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// IssueCreateRequest is the request body for creating an issue.
type IssueCreateRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    int    `json:"priority,omitempty"` // 1-4, default 2
}

// IssueCreateResponse is the response from creating an issue.
type IssueCreateResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleIssueCreate creates a new issue via bd create.
func (h *APIHandler) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	var req IssueCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		h.sendError(w, "Title is required", http.StatusBadRequest)
		return
	}

	// Enforce length limits to prevent oversized payloads
	const maxTitleLen = 500
	const maxDescriptionLen = 100_000 // 100KB
	if len(req.Title) > maxTitleLen {
		h.sendError(w, fmt.Sprintf("Title too long (max %d bytes)", maxTitleLen), http.StatusBadRequest)
		return
	}
	if len(req.Description) > maxDescriptionLen {
		h.sendError(w, fmt.Sprintf("Description too long (max %d bytes)", maxDescriptionLen), http.StatusBadRequest)
		return
	}

	// Validate title doesn't contain control characters or newlines
	if strings.ContainsAny(req.Title, "\n\r\x00") {
		h.sendError(w, "Title cannot contain newlines or control characters", http.StatusBadRequest)
		return
	}

	// Validate description if provided
	if req.Description != "" && strings.Contains(req.Description, "\x00") {
		h.sendError(w, "Description cannot contain null characters", http.StatusBadRequest)
		return
	}

	// Build bd create command. Flags go first, then -- to end flag parsing,
	// then the positional title (prevents titles like "--help" being parsed as flags).
	// bd uses cobra/pflag which respects -- natively (verified: bd --help shows cobra format).
	args := []string{"create"}

	// Add priority if specified (default is P2)
	if req.Priority >= 1 && req.Priority <= 4 {
		args = append(args, fmt.Sprintf("--priority=%d", req.Priority))
	}

	// Add description if provided
	if req.Description != "" {
		args = append(args, "--body", req.Description)
	}

	args = append(args, "--", req.Title)

	// Run bd create
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	output, err := h.runBdCommand(ctx, 12*time.Second, args)

	resp := IssueCreateResponse{}
	if err != nil {
		resp.Success = false
		resp.Error = "Failed to create issue: " + err.Error()
		if output != "" {
			resp.Message = output
		}
	} else {
		resp.Success = true
		resp.Message = output

		// Try to extract issue ID from output (e.g., "Created issue: abc123")
		if strings.Contains(output, "Created") {
			parts := strings.Fields(output)
			for i, p := range parts {
				if strings.HasSuffix(p, ":") && i+1 < len(parts) {
					resp.ID = strings.TrimSpace(parts[i+1])
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// IssueCloseRequest is the request body for closing an issue.
type IssueCloseRequest struct {
	ID string `json:"id"`
}

// handleIssueClose closes an issue via bd close.
func (h *APIHandler) handleIssueClose(w http.ResponseWriter, r *http.Request) {
	var req IssueCloseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		h.sendError(w, "Issue ID is required", http.StatusBadRequest)
		return
	}
	if !isValidID(req.ID) {
		h.sendError(w, "Invalid issue ID format", http.StatusBadRequest)
		return
	}

	output, err := h.runBdCommand(r.Context(), 12*time.Second, []string{"close", req.ID})

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to close issue: " + err.Error(),
			"output":  output,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Issue closed",
		"output":  output,
	})
}

// IssueUpdateRequest is the request body for updating an issue.
type IssueUpdateRequest struct {
	ID       string `json:"id"`
	Status   string `json:"status,omitempty"`   // "open", "in_progress"
	Priority int    `json:"priority,omitempty"` // 1-4
	Assignee string `json:"assignee,omitempty"`
}

// handleIssueUpdate updates issue fields via bd update.
func (h *APIHandler) handleIssueUpdate(w http.ResponseWriter, r *http.Request) {
	var req IssueUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		h.sendError(w, "Issue ID is required", http.StatusBadRequest)
		return
	}
	if !isValidID(req.ID) {
		h.sendError(w, "Invalid issue ID format", http.StatusBadRequest)
		return
	}

	// Build bd update args
	args := []string{"update", req.ID}
	hasUpdate := false

	if req.Status != "" {
		// Validate allowed status values
		switch req.Status {
		case "open", "in_progress":
			args = append(args, "--status="+req.Status)
			hasUpdate = true
		default:
			h.sendError(w, "Invalid status (allowed: open, in_progress)", http.StatusBadRequest)
			return
		}
	}

	if req.Priority >= 1 && req.Priority <= 4 {
		args = append(args, fmt.Sprintf("--priority=%d", req.Priority))
		hasUpdate = true
	}

	if req.Assignee != "" {
		if !isValidID(req.Assignee) {
			h.sendError(w, "Invalid assignee format", http.StatusBadRequest)
			return
		}
		args = append(args, "--assignee="+req.Assignee)
		hasUpdate = true
	}

	if !hasUpdate {
		h.sendError(w, "No update fields provided", http.StatusBadRequest)
		return
	}

	output, err := h.runBdCommand(r.Context(), 12*time.Second, args)

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to update issue: " + err.Error(),
			"output":  output,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Issue updated",
		"output":  output,
	})
}

// parseIssueShowJSON parses the JSON output from "bd show <id> --json".
// Returns (response, true) on success, or (zero, false) if parsing fails.
func parseIssueShowJSON(output string) (IssueShowResponse, bool) {
	var items []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Priority    int      `json:"priority"`
		Type        string   `json:"issue_type"`
		Owner       string   `json:"owner"`
		CreatedAt   string   `json:"created_at"`
		UpdatedAt   string   `json:"updated_at"`
		DependsOn   []string `json:"depends_on,omitempty"`
		Blocks      []string `json:"blocks,omitempty"`
	}
	if err := json.Unmarshal([]byte(output), &items); err != nil || len(items) == 0 {
		return IssueShowResponse{}, false
	}
	item := items[0]

	priority := ""
	if item.Priority > 0 {
		priority = fmt.Sprintf("P%d", item.Priority)
	}

	return IssueShowResponse{
		ID:          item.ID,
		Title:       item.Title,
		Type:        item.Type,
		Status:      item.Status,
		Priority:    priority,
		Owner:       item.Owner,
		Description: item.Description,
		Created:     item.CreatedAt,
		Updated:     item.UpdatedAt,
		DependsOn:   item.DependsOn,
		Blocks:      item.Blocks,
		RawOutput:   output,
	}, true
}

// parseIssueShowOutput parses the text output from "bd show <id>".
// This is the fallback path when --json is unavailable.
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
		if !parsedFirstLine && (strings.HasPrefix(line, "\u25cb") || strings.HasPrefix(line, "\u25cf")) {
			parsedFirstLine = true
			// Parse the first line for title and status
			// Format: "○ id · title   [● P2 · OPEN]"
			// Find the bracket first to isolate the status
			if bracketIdx := strings.Index(line, "["); bracketIdx > 0 {
				beforeBracket := line[:bracketIdx]
				statusPart := line[bracketIdx:]

				// Extract priority and status from [● P2 · OPEN]
				statusPart = strings.Trim(statusPart, "[]\u25cf\u25cb ")
				statusParts := strings.Split(statusPart, "\u00b7")
				if len(statusParts) >= 1 {
					resp.Priority = strings.TrimSpace(statusParts[0])
				}
				if len(statusParts) >= 2 {
					resp.Status = strings.TrimSpace(statusParts[1])
				}

				// Now parse the title from before the bracket
				// Format: "○ id · title"
				// Use strings.Cut for safe splitting on multi-byte "·" separator
				if _, afterFirst, ok := strings.Cut(beforeBracket, "\u00b7"); ok {
					if _, afterSecond, ok := strings.Cut(afterFirst, "\u00b7"); ok {
						resp.Title = strings.TrimSpace(afterSecond)
					} else {
						// Only one dot - id is embedded in icon part
						resp.Title = strings.TrimSpace(afterFirst)
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "Owner:") {
			// Format: "Owner: mayor · Type: task"
			ownerLine := strings.TrimPrefix(line, "Owner:")
			ownerParts := strings.Split(ownerLine, "\u00b7")
			resp.Owner = strings.TrimSpace(ownerParts[0])
			if len(ownerParts) >= 2 {
				typePart := strings.TrimSpace(ownerParts[1])
				resp.Type = strings.TrimSpace(strings.TrimPrefix(typePart, "Type:"))
			}
		} else if strings.HasPrefix(line, "Type:") {
			resp.Type = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		} else if strings.HasPrefix(line, "Created:") {
			// Split always returns >= 1 element; parts[0] is safe unconditionally
			parts := strings.Split(line, "\u00b7")
			resp.Created = strings.TrimSpace(strings.TrimPrefix(parts[0], "Created:"))
			if len(parts) >= 2 {
				resp.Updated = strings.TrimSpace(strings.TrimPrefix(parts[1], "Updated:"))
			}
		} else if line == "DESCRIPTION" {
			inDescription = true
		} else if line == "DEPENDS ON" || line == "BLOCKS" {
			inDescription = false
		} else if inDescription && strings.TrimSpace(line) != "" {
			descLines = append(descLines, line)
		} else if strings.HasPrefix(strings.TrimSpace(line), "\u2192") {
			// Dependency line
			depLine := strings.TrimSpace(line)
			depLine = strings.TrimPrefix(depLine, "\u2192")
			depLine = strings.TrimSpace(depLine)
			// Extract just the bead ID
			if colonIdx := strings.Index(depLine, ":"); colonIdx > 0 {
				parts := strings.Fields(depLine[:colonIdx])
				if len(parts) >= 2 {
					dependsOn = append(dependsOn, parts[1])
				}
			}
		} else if strings.HasPrefix(strings.TrimSpace(line), "\u2190") {
			// Blocks line
			blockLine := strings.TrimSpace(line)
			blockLine = strings.TrimPrefix(blockLine, "\u2190")
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

	// Validate inputs to prevent argument injection.
	// When url is provided, repo/number are ignored — only validate what's used.
	if prURL != "" {
		const maxURLLen = 2000
		if len(prURL) > maxURLLen {
			h.sendError(w, fmt.Sprintf("PR URL too long (max %d bytes)", maxURLLen), http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(prURL, "\x00\n\r") {
			h.sendError(w, "PR URL cannot contain null bytes or newlines", http.StatusBadRequest)
			return
		}
		// Allow any https:// URL, not just github.com — supports GitHub Enterprise.
		// gh CLI validates against the configured host and rejects non-GitHub API responses,
		// limiting SSRF risk. Localhost-only deployment further reduces exposure.
		if !strings.HasPrefix(prURL, "https://") {
			h.sendError(w, "PR URL must start with https://", http.StatusBadRequest)
			return
		}
	} else {
		if !isNumeric(number) {
			h.sendError(w, "Invalid PR number format", http.StatusBadRequest)
			return
		}
		if !isValidRepoRef(repo) {
			h.sendError(w, "Invalid repo format (expected owner/repo)", http.StatusBadRequest)
			return
		}
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

// parsePRShowOutput parses the JSON output from "gh pr view --json".
func parsePRShowOutput(jsonStr string) PRShowResponse {
	resp := PRShowResponse{
		RawOutput: jsonStr,
	}

	var data struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Author struct {
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
