package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/outdoorsea/faultline/internal/db"
)

// ProjectInfo holds project configuration needed by SDK clients.
type ProjectInfo struct {
	ID        int64  `json:"id"`
	PublicKey string `json:"public_key"`
	Rig       string `json:"rig,omitempty"`
}

// Handler serves the faultline read/management API.
type Handler struct {
	DB       *db.DB
	Log      *slog.Logger
	Projects []ProjectInfo // populated from FAULTLINE_PROJECTS config
	BaseURL  string        // e.g. "http://localhost:8080" for DSN generation
}

// RegisterRoutes adds API routes to the given mux.
// All routes except /api/hooks/* require Bearer token authentication.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	auth := h.requireBearer

	// Project listing — uses /api/v1/projects to avoid conflict with {project_id} wildcard.
	mux.HandleFunc("GET /api/v1/projects/", auth(h.listProjects))
	mux.HandleFunc("GET /api/v1/projects", auth(h.listProjects))

	// Issue CRUD.
	mux.HandleFunc("GET /api/{project_id}/issues/", auth(h.listIssues))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/", auth(h.getIssue))
	mux.HandleFunc("PUT /api/{project_id}/issues/{issue_id}/", auth(h.updateIssue))
	mux.HandleFunc("PATCH /api/{project_id}/issues/{issue_id}/", auth(h.updateIssue))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/events/", auth(h.listEvents))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/context/", auth(h.getIssueContext))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/dolt-log/", auth(h.getDoltLog))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign-bead/", auth(h.assignBead))
	mux.HandleFunc("GET /api/{project_id}/events/{event_id}/", auth(h.getEvent))
	mux.HandleFunc("GET /api/{project_id}/dsn/", auth(h.getProjectDSN))

	// Internal hooks (no Bearer auth — called by Gas Town with shared secret).
	mux.HandleFunc("POST /api/hooks/resolve/", h.resolveHook)

	// API token management.
	h.registerTokenRoutes(mux)

	// Without trailing slash variants.
	mux.HandleFunc("GET /api/{project_id}/issues", auth(h.listIssues))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}", auth(h.getIssue))
	mux.HandleFunc("PUT /api/{project_id}/issues/{issue_id}", auth(h.updateIssue))
	mux.HandleFunc("PATCH /api/{project_id}/issues/{issue_id}", auth(h.updateIssue))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/events", auth(h.listEvents))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/context", auth(h.getIssueContext))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/dolt-log", auth(h.getDoltLog))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign-bead", auth(h.assignBead))
	mux.HandleFunc("GET /api/{project_id}/events/{event_id}", auth(h.getEvent))
	mux.HandleFunc("GET /api/{project_id}/dsn", auth(h.getProjectDSN))
	mux.HandleFunc("POST /api/hooks/resolve", h.resolveHook)
}

// requireBearer wraps a handler with Bearer token authentication.
// Accepts tokens from: "Authorization: Bearer <token>" header.
// Validates against session tokens first, then API tokens (fl_ prefix).
func (h *Handler) requireBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="faultline"`)
			writeErr(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		// Try session token first (dashboard login tokens).
		account, err := h.DB.GetSession(r.Context(), token)
		if err == nil && account != nil {
			next(w, r)
			return
		}

		// Try API token (fl_ prefixed tokens for agents/CI).
		account, err = h.DB.ValidateAPIToken(r.Context(), token)
		if err == nil && account != nil {
			next(w, r)
			return
		}

		w.Header().Set("WWW-Authenticate", `Bearer realm="faultline"`)
		writeErr(w, http.StatusUnauthorized, "invalid or expired token")
	}
}

// extractBearerToken extracts a Bearer token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(auth[len(prefix):])
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.DB.ListProjects(r.Context())
	if err != nil {
		h.Log.Error("list projects", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projects,
	})
}

func (h *Handler) listIssues(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	params := db.IssueListParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		Level:     r.URL.Query().Get("level"),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
		Limit:     queryInt(r, "limit", 25),
		Offset:    queryInt(r, "offset", 0),
		Query:     r.URL.Query().Get("query"),
	}

	issues, total, err := h.DB.ListIssueGroups(r.Context(), params)
	if err != nil {
		h.Log.Error("list issues", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issues": issues,
		"total":  total,
	})
}

func (h *Handler) getIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) updateIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	switch body.Status {
	case "resolved":
		if err := h.DB.ResolveIssueGroup(r.Context(), projectID, issueID); err != nil {
			h.Log.Error("resolve issue", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	case "unresolved":
		if err := h.DB.UnresolveIssueGroup(r.Context(), projectID, issueID); err != nil {
			h.Log.Error("unresolve issue", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	case "ignored":
		if err := h.DB.IgnoreIssueGroup(r.Context(), projectID, issueID); err != nil {
			h.Log.Error("ignore issue", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "status must be 'resolved', 'unresolved', or 'ignored'")
		return
	}

	// Return the updated issue.
	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	limit := queryInt(r, "limit", 25)
	offset := queryInt(r, "offset", 0)

	events, err := h.DB.ListEventsByGroup(r.Context(), projectID, issueID, limit, offset)
	if err != nil {
		h.Log.Error("list events", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
	})
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	eventID := r.PathValue("event_id")
	if projectID <= 0 || eventID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	event, err := h.DB.GetEvent(r.Context(), projectID, eventID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "event not found")
		return
	}

	writeJSON(w, http.StatusOK, event)
}

// getIssueContext returns a polecat-friendly context bundle for an issue group.
// Includes the issue summary, recent events with stack traces, and resolution status.
func (h *Handler) getIssueContext(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}

	// Fetch the 5 most recent events for stack trace context.
	events, err := h.DB.ListEventsByGroup(r.Context(), projectID, issueID, 5, 0)
	if err != nil {
		h.Log.Error("list events for context", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Extract stack traces from raw event JSON for easier consumption.
	type stackFrame struct {
		Filename string `json:"filename,omitempty"`
		Function string `json:"function,omitempty"`
		Lineno   int    `json:"lineno,omitempty"`
		Colno    int    `json:"colno,omitempty"`
		Module   string `json:"module,omitempty"`
	}
	type exceptionEntry struct {
		Type       string       `json:"type"`
		Value      string       `json:"value"`
		Stacktrace []stackFrame `json:"stacktrace,omitempty"`
	}
	type eventSummary struct {
		EventID    string           `json:"event_id"`
		Timestamp  string           `json:"timestamp"`
		Platform   string           `json:"platform"`
		Level      string           `json:"level"`
		Message    string           `json:"message"`
		Exceptions []exceptionEntry `json:"exceptions,omitempty"`
	}

	var eventSummaries []eventSummary
	for _, ev := range events {
		summary := eventSummary{
			EventID:   ev.EventID,
			Timestamp: ev.Timestamp.Format("2006-01-02T15:04:05Z"),
			Platform:  ev.Platform,
			Level:     ev.Level,
			Message:   ev.Message,
		}

		// Parse exceptions from raw JSON.
		var raw struct {
			Exception *struct {
				Values []struct {
					Type       string `json:"type"`
					Value      string `json:"value"`
					Stacktrace *struct {
						Frames []struct {
							Filename string `json:"filename"`
							Function string `json:"function"`
							Lineno   int    `json:"lineno"`
							Colno    int    `json:"colno"`
							Module   string `json:"module"`
						} `json:"frames"`
					} `json:"stacktrace"`
				} `json:"values"`
			} `json:"exception"`
		}
		if err := json.Unmarshal(ev.RawJSON, &raw); err == nil && raw.Exception != nil {
			for _, exc := range raw.Exception.Values {
				entry := exceptionEntry{Type: exc.Type, Value: exc.Value}
				if exc.Stacktrace != nil {
					for _, f := range exc.Stacktrace.Frames {
						entry.Stacktrace = append(entry.Stacktrace, stackFrame{
							Filename: f.Filename,
							Function: f.Function,
							Lineno:   f.Lineno,
							Colno:    f.Colno,
							Module:   f.Module,
						})
					}
				}
				summary.Exceptions = append(summary.Exceptions, entry)
			}
		}

		eventSummaries = append(eventSummaries, summary)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issue":  issue,
		"events": eventSummaries,
	})
}

// getDoltLog returns the Dolt commit history for a specific issue group.
// This is a unique faultline feature: every data change is tracked as a Dolt commit,
// providing a full audit trail of when an issue was created, updated, resolved, etc.
func (h *Handler) getDoltLog(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	limit := queryInt(r, "limit", 20)

	entries, err := h.DB.DoltLogForIssue(r.Context(), projectID, issueID, limit)
	if err != nil {
		h.Log.Error("dolt log", "err", err, "project_id", projectID, "issue_id", issueID)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"commits": entries,
	})
}

// assignBead handles POST /api/{project_id}/issues/{issue_id}/assign-bead —
// called by polecats to associate a Gas Town bead with an issue group.
func (h *Handler) assignBead(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	var body struct {
		BeadID string `json:"bead_id"`
		Rig    string `json:"rig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.BeadID == "" {
		writeErr(w, http.StatusBadRequest, "bead_id is required")
		return
	}

	// Verify the issue exists.
	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}

	if err := h.DB.InsertBead(r.Context(), issueID, projectID, body.BeadID, body.Rig); err != nil {
		h.Log.Error("assign bead", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("bead assigned to issue",
		"project_id", projectID,
		"issue_id", issueID,
		"bead_id", body.BeadID,
	)

	// Return updated issue with bead_id.
	issue, err = h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

// resolveHook handles POST /api/hooks/resolve — called by Gas Town (refinery)
// after a fix is merged to mark the faultline issue as resolved.
func (h *Handler) resolveHook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID int64  `json:"project_id"`
		GroupID   string `json:"group_id"`
		BeadID    string `json:"bead_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.ProjectID <= 0 || body.GroupID == "" {
		writeErr(w, http.StatusBadRequest, "project_id and group_id required")
		return
	}

	if err := h.DB.MarkBeadResolved(r.Context(), body.ProjectID, body.GroupID); err != nil {
		h.Log.Error("resolve hook", "err", err, "project_id", body.ProjectID, "group_id", body.GroupID)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("issue resolved via hook",
		"project_id", body.ProjectID,
		"group_id", body.GroupID,
		"bead_id", body.BeadID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

// getProjectDSN returns the Sentry DSN and configuration for a project.
// Used by SDK clients (e.g. myndy_ios) to discover their DSN URL.
func (h *Handler) getProjectDSN(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	for _, p := range h.Projects {
		if p.ID == projectID {
			// DSN format: {protocol}://{public_key}@{host}/{project_id}
			dsn := fmt.Sprintf("%s/%d", h.BaseURL, p.ID)
			if h.BaseURL != "" && p.PublicKey != "" {
				if u, err := url.Parse(h.BaseURL); err == nil {
					dsn = fmt.Sprintf("%s://%s@%s/%d", u.Scheme, p.PublicKey, u.Host, p.ID)
				}
			}

			writeJSON(w, http.StatusOK, map[string]interface{}{
				"project_id": p.ID,
				"public_key": p.PublicKey,
				"rig":        p.Rig,
				"dsn":        dsn,
				"endpoints": map[string]string{
					"envelope": fmt.Sprintf("%s/api/%d/envelope/", h.BaseURL, p.ID),
					"store":    fmt.Sprintf("%s/api/%d/store/", h.BaseURL, p.ID),
				},
			})
			return
		}
	}

	writeErr(w, http.StatusNotFound, "project not found")
}

// Helpers

func pathInt64(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return v
}

func queryInt(r *http.Request, name string, fallback int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
