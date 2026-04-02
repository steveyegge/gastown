package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/slackdm"
)

// Handler serves the faultline read/management API.
type Handler struct {
	DB            *db.DB
	Log           *slog.Logger
	BaseURL       string           // e.g. "http://localhost:8080" for DSN generation
	Auth          ProjectRegistrar // for dynamic project registration
	HookSecret    string           // HMAC secret for webhook verification (resolve hook)
	SlackDMs      *slackdm.Sender // optional; sends Slack DMs for mentions/assignments
	EncryptionKey []byte           // AES-256 key for encrypting connection strings
}

// ProjectRegistrar allows the API to register and unregister projects at runtime.
type ProjectRegistrar interface {
	Register(publicKey string, projectID int64, rig string)
	Unregister(publicKey string, projectID int64)
	UnregisterAll()
}

// RegisterRoutes adds API routes to the given mux.
// All routes except /api/hooks/* require Bearer token authentication.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	member := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("member", h) }
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }
	owner := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("owner", h) }

	// Project listing, registration, and removal.
	mux.HandleFunc("GET /api/v1/projects/", auth(h.listProjects))
	mux.HandleFunc("GET /api/v1/projects", auth(h.listProjects))
	mux.HandleFunc("POST /api/v1/register", auth(owner(h.registerProject)))
	mux.HandleFunc("POST /api/v1/register/", auth(owner(h.registerProject)))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}", auth(owner(h.unregisterProject)))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/", auth(owner(h.unregisterProject)))
	mux.HandleFunc("POST /api/v1/reset-projects", auth(owner(h.resetProjects)))
	mux.HandleFunc("POST /api/v1/reset-projects/", auth(owner(h.resetProjects)))

	// Issue CRUD.
	mux.HandleFunc("GET /api/{project_id}/issues/", auth(projAccess(h.listIssues)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/", auth(projAccess(h.getIssue)))
	mux.HandleFunc("PUT /api/{project_id}/issues/{issue_id}/", auth(projAccess(member(h.updateIssue))))
	mux.HandleFunc("PATCH /api/{project_id}/issues/{issue_id}/", auth(projAccess(member(h.updateIssue))))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/events/", auth(projAccess(h.listEvents)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/context/", auth(projAccess(h.getIssueContext)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/dolt-log/", auth(projAccess(h.getDoltLog)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/history/", auth(projAccess(h.getIssueHistory)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/as-of/", auth(projAccess(h.getIssueAsOf)))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign-bead/", auth(projAccess(member(h.assignBead))))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/lifecycle/", auth(projAccess(h.getLifecycle)))
	mux.HandleFunc("GET /api/{project_id}/events/{event_id}/", auth(projAccess(h.getEvent)))
	mux.HandleFunc("GET /api/{project_id}/releases/", auth(projAccess(h.listReleases)))
	mux.HandleFunc("GET /api/{project_id}/releases/{version}/", auth(projAccess(h.getRelease)))
	mux.HandleFunc("POST /api/{project_id}/deploys/", auth(projAccess(member(h.registerDeploy))))
	mux.HandleFunc("GET /api/{project_id}/dsn/", auth(projAccess(h.getProjectDSN)))

	// Snooze/unsnooze.
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/snooze/", auth(projAccess(member(h.snoozeIssue))))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/unsnooze/", auth(projAccess(member(h.unsnoozeIssue))))

	// Bulk operations.
	mux.HandleFunc("POST /api/{project_id}/bulk-issues/", auth(projAccess(member(h.bulkOperateIssues))))

	// Comments.
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/comments/", auth(projAccess(h.listComments)))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/comments/", auth(projAccess(member(h.createComment))))

	// Alert rules CRUD.
	mux.HandleFunc("GET /api/{project_id}/alert-rules/", auth(projAccess(h.listAlertRules)))
	mux.HandleFunc("POST /api/{project_id}/alert-rules/", auth(projAccess(admin(h.createAlertRule))))
	mux.HandleFunc("GET /api/{project_id}/alert-rules/{rule_id}/", auth(projAccess(h.getAlertRule)))
	mux.HandleFunc("PUT /api/{project_id}/alert-rules/{rule_id}/", auth(projAccess(admin(h.updateAlertRule))))
	mux.HandleFunc("DELETE /api/{project_id}/alert-rules/{rule_id}/", auth(projAccess(admin(h.deleteAlertRule))))
	mux.HandleFunc("GET /api/{project_id}/alert-history/", auth(projAccess(h.listAlertHistory)))

	// Internal hooks (no Bearer auth — called by Gas Town with shared secret).
	mux.HandleFunc("POST /api/hooks/resolve/", h.resolveHook)

	// API token management.
	h.registerTokenRoutes(mux)

	// Fingerprint rules management.
	h.registerFingerprintRuleRoutes(mux)

	// Source map upload/list/delete.
	h.registerSourceMapRoutes(mux)

	// Integration configs CRUD.
	h.registerIntegrationRoutes(mux)

	// Webhook template management.
	h.registerWebhookTemplateRoutes(mux)

	// Issue merge/unmerge.
	h.registerMergeRoutes(mux)

	// Team management.
	h.registerTeamRoutes(mux)

	// Issue assignment.
	h.registerAssignmentRoutes(mux)

	// Database monitoring CRUD.
	h.registerDatabaseRoutes(mux)

	// Without trailing slash variants.
	mux.HandleFunc("GET /api/{project_id}/issues", auth(projAccess(h.listIssues)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}", auth(projAccess(h.getIssue)))
	mux.HandleFunc("PUT /api/{project_id}/issues/{issue_id}", auth(projAccess(member(h.updateIssue))))
	mux.HandleFunc("PATCH /api/{project_id}/issues/{issue_id}", auth(projAccess(member(h.updateIssue))))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/events", auth(projAccess(h.listEvents)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/context", auth(projAccess(h.getIssueContext)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/dolt-log", auth(projAccess(h.getDoltLog)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/history", auth(projAccess(h.getIssueHistory)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/as-of", auth(projAccess(h.getIssueAsOf)))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign-bead", auth(projAccess(member(h.assignBead))))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/lifecycle", auth(projAccess(h.getLifecycle)))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/snooze", auth(projAccess(member(h.snoozeIssue))))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/unsnooze", auth(projAccess(member(h.unsnoozeIssue))))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/comments", auth(projAccess(h.listComments)))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/comments", auth(projAccess(member(h.createComment))))
	mux.HandleFunc("GET /api/{project_id}/events/{event_id}", auth(projAccess(h.getEvent)))
	mux.HandleFunc("GET /api/{project_id}/releases", auth(projAccess(h.listReleases)))
	mux.HandleFunc("GET /api/{project_id}/releases/{version}", auth(projAccess(h.getRelease)))
	mux.HandleFunc("POST /api/{project_id}/deploys", auth(projAccess(member(h.registerDeploy))))
	mux.HandleFunc("GET /api/{project_id}/dsn", auth(projAccess(h.getProjectDSN)))
	mux.HandleFunc("GET /api/{project_id}/alert-rules", auth(projAccess(h.listAlertRules)))
	mux.HandleFunc("POST /api/{project_id}/alert-rules", auth(projAccess(admin(h.createAlertRule))))
	mux.HandleFunc("GET /api/{project_id}/alert-rules/{rule_id}", auth(projAccess(h.getAlertRule)))
	mux.HandleFunc("PUT /api/{project_id}/alert-rules/{rule_id}", auth(projAccess(admin(h.updateAlertRule))))
	mux.HandleFunc("DELETE /api/{project_id}/alert-rules/{rule_id}", auth(projAccess(admin(h.deleteAlertRule))))
	mux.HandleFunc("GET /api/{project_id}/alert-history", auth(projAccess(h.listAlertHistory)))
	mux.HandleFunc("POST /api/hooks/resolve", h.resolveHook)
}

// requireBearer wraps a handler with Bearer token authentication.
// Accepts tokens from: "Authorization: Bearer <token>" header.
// Validates against session tokens first, then API tokens (fl_ prefix).
// Sets X-Account-ID and X-Account-Role headers for downstream handlers.
// For API tokens, the effective role is the lower of account and token roles.
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
		if err != nil || account == nil {
			// Try API token (fl_ prefixed tokens for agents/CI).
			tr, apiErr := h.DB.ValidateAPIToken(r.Context(), token)
			if apiErr != nil || tr == nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="faultline"`)
				writeErr(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			r.Header.Set("X-Account-ID", formatInt64(tr.Account.ID))
			r.Header.Set("X-Account-Role", effectiveRole(tr.Account.Role, tr.TokenRole))
			if tr.ProjectID != nil {
				r.Header.Set("X-Token-Project-ID", formatInt64(*tr.ProjectID))
			}
			next(w, r)
			return
		}

		r.Header.Set("X-Account-ID", formatInt64(account.ID))
		r.Header.Set("X-Account-Role", account.Role)
		next(w, r)
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

	// Filter projects by team-based access for non-admin users.
	accountID := headerAccountID(r)
	accountRole := headerAccountRole(r)
	allowed, err := h.DB.ProjectIDsVisibleTo(r.Context(), accountID, accountRole)
	if err != nil {
		h.Log.Error("project visibility check", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if allowed != nil {
		filtered := make([]db.Project, 0, len(allowed))
		allowSet := make(map[int64]bool, len(allowed))
		for _, id := range allowed {
			allowSet[id] = true
		}
		for _, p := range projects {
			if allowSet[p.ID] {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projects,
	})
}

func (h *Handler) registerProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`     // project name (required)
		Rig      string `json:"rig"`      // Gas Town rig name (optional, defaults to name)
		Language string `json:"language"` // python, node, go, swift (optional, for setup snippets)
		URL      string `json:"url"`      // project web URL (optional, e.g. "http://localhost:3000")
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Rig == "" {
		req.Rig = req.Name
	}

	// Generate a random public key.
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		writeErr(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	publicKey := fmt.Sprintf("%x", keyBytes)

	// Find next available project ID.
	projects, err := h.DB.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	var maxID int64
	for _, p := range projects {
		if p.ID > maxID {
			maxID = p.ID
		}
	}
	projectID := maxID + 1

	// Create in database.
	if err := h.DB.EnsureProject(r.Context(), projectID, req.Name, req.Name, publicKey); err != nil {
		h.Log.Error("register project", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	// Store project config (URL, etc.) if provided.
	if req.URL != "" {
		_ = h.DB.UpdateProjectConfig(r.Context(), projectID, &db.ProjectConfig{URL: req.URL})
	}

	// Register in auth (live, no restart needed).
	if h.Auth != nil {
		h.Auth.Register(publicKey, projectID, req.Rig)
	}

	baseURL := h.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	dsn := fmt.Sprintf("%s/%s@%s/%d",
		strings.TrimSuffix(baseURL, "/"),
		publicKey,
		strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://"),
		projectID,
	)
	// Build proper Sentry DSN format: http://key@host:port/project_id
	host := strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://"), "https://")
	scheme := "http"
	if strings.HasPrefix(baseURL, "https://") {
		scheme = "https"
	}
	sentryDSN := fmt.Sprintf("%s://%s@%s/%d", scheme, publicKey, host, projectID)

	h.Log.Info("project registered",
		"id", projectID,
		"name", req.Name,
		"rig", req.Rig,
		"key", publicKey[:8]+"...",
	)

	resp := map[string]interface{}{
		"project_id": projectID,
		"name":       req.Name,
		"rig":        req.Rig,
		"public_key": publicKey,
		"dsn":        sentryDSN,
		"endpoints": map[string]string{
			"envelope":  fmt.Sprintf("%s/api/%d/envelope/", baseURL, projectID),
			"store":     fmt.Sprintf("%s/api/%d/store/", baseURL, projectID),
			"heartbeat": fmt.Sprintf("%s/api/%d/heartbeat", baseURL, projectID),
		},
		"env_var": fmt.Sprintf("FAULTLINE_DSN=%s", sentryDSN),
		"setup":   setupSnippet(req.Language, sentryDSN),
		"notes": []string{
			"Set traces_sample_rate=0 — faultline only processes errors",
			"Add heartbeat for liveness detection (see setup snippet)",
			"FAULTLINE_DSN and SENTRY_DSN both work — use FAULTLINE_DSN to avoid confusion",
			"See docs/INTEGRATION.md for full guide",
		},
	}

	writeJSON(w, http.StatusCreated, resp)
	_ = dsn // suppress unused
}

func (h *Handler) unregisterProject(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	if err := h.DB.DeleteProject(r.Context(), projectID); err != nil {
		h.Log.Error("unregister project", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	if h.Auth != nil {
		h.Auth.Unregister(project.DSNPublicKey, projectID)
	}

	h.Log.Info("project unregistered", "id", projectID, "name", project.Name)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "deleted",
		"project_id": projectID,
		"name":       project.Name,
	})
}

func (h *Handler) resetProjects(w http.ResponseWriter, r *http.Request) {
	count, err := h.DB.DeleteAllProjects(r.Context())
	if err != nil {
		h.Log.Error("reset projects", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to reset projects")
		return
	}

	if h.Auth != nil {
		h.Auth.UnregisterAll()
	}

	h.Log.Info("all projects reset", "deleted", count)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "reset",
		"deleted": count,
	})
}

func setupSnippet(language, dsn string) string {
	switch strings.ToLower(language) {
	case "python":
		return fmt.Sprintf(`pip install sentry-sdk[fastapi]

import sentry_sdk, os
sentry_sdk.init(
    dsn=os.environ.get("FAULTLINE_DSN", "%s"),
    traces_sample_rate=0,
    enable_tracing=False,
)

# Add heartbeat (see docs/HEARTBEAT.md)`, dsn)
	case "node", "javascript", "typescript":
		return fmt.Sprintf(`npm install @sentry/node

import * as Sentry from "@sentry/node";
Sentry.init({
  dsn: process.env.FAULTLINE_DSN || "%s",
  tracesSampleRate: 0,
});

// Add heartbeat (see docs/HEARTBEAT.md)`, dsn)
	case "go":
		return fmt.Sprintf(`go get github.com/outdoorsea/faultline/pkg/gtfaultline

gtfaultline.Init(gtfaultline.Config{
    DSN: os.Getenv("FAULTLINE_DSN"), // %s
})
defer gtfaultline.Flush(2 * time.Second)
// Heartbeat is automatic`, dsn)
	case "swift", "ios":
		return fmt.Sprintf(`// Add sentry-cocoa via SPM or CocoaPods
import Sentry

SentrySDK.start { options in
    options.dsn = "%s"
    options.enableAutoSessionTracking = true
    options.attachStacktrace = true
}

// Add heartbeat (see docs/HEARTBEAT.md)`, dsn)
	default:
		return fmt.Sprintf("Set FAULTLINE_DSN=%s and install the Sentry SDK for your language. See docs/INTEGRATION.md", dsn)
	}
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
		Platform:  r.URL.Query().Get("platform"),
		Sort:      r.URL.Query().Get("sort"),
		Order:     r.URL.Query().Get("order"),
		Limit:     queryInt(r, "limit", 25),
		Offset:    queryInt(r, "offset", 0),
		Query:     r.URL.Query().Get("query"),
		Release:   r.URL.Query().Get("release"),
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
		Status         string `json:"status"`
		RootCause      string `json:"root_cause"`
		FixExplanation string `json:"fix_explanation"`
		FixCommit      string `json:"fix_commit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Update resolution fields if provided.
	if body.RootCause != "" || body.FixExplanation != "" || body.FixCommit != "" {
		if err := h.DB.UpdateIssueResolution(r.Context(), projectID, issueID, body.RootCause, body.FixExplanation, body.FixCommit); err != nil {
			h.Log.Error("update issue resolution", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if body.Status != "" {
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
		case "regressed":
			if err := h.DB.RegressIssueGroup(r.Context(), projectID, issueID); err != nil {
				h.Log.Error("regress issue", "err", err)
				writeErr(w, http.StatusInternalServerError, "internal error")
				return
			}
		case "snoozed":
			writeErr(w, http.StatusBadRequest, "use POST /api/{project_id}/issues/{issue_id}/snooze to snooze an issue")
			return
		default:
			writeErr(w, http.StatusBadRequest, "status must be 'resolved', 'unresolved', 'ignored', or 'regressed'")
			return
		}
	}

	// Return the updated issue.
	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) snoozeIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	var body struct {
		DurationSeconds int    `json:"duration_seconds"`
		Reason          string `json:"reason"`
		SnoozedBy       string `json:"snoozed_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.DurationSeconds <= 0 {
		writeErr(w, http.StatusBadRequest, "duration_seconds must be positive")
		return
	}
	if body.SnoozedBy == "" {
		body.SnoozedBy = "api"
	}

	duration := time.Duration(body.DurationSeconds) * time.Second
	if err := h.DB.SnoozeIssueGroup(r.Context(), projectID, issueID, body.Reason, body.SnoozedBy, duration); err != nil {
		h.Log.Error("snooze issue", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleSnoozed, nil, nil, map[string]interface{}{
		"trigger":          "api",
		"reason":           body.Reason,
		"duration_seconds": body.DurationSeconds,
		"snoozed_by":       body.SnoozedBy,
	})

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) unsnoozeIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.UnsnoozeIssueGroup(r.Context(), projectID, issueID); err != nil {
		h.Log.Error("unsnooze issue", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleUnsnoozed, nil, nil, map[string]interface{}{
		"trigger": "api",
	})

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (h *Handler) bulkOperateIssues(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var body struct {
		IssueIDs        []string `json:"issue_ids"`
		Action          string   `json:"action"`           // "resolve", "ignore", "unresolve", "assign", "snooze", "unsnooze"
		AssignedTo      string   `json:"assigned_to"`      // required for "assign"
		AssignedBy      string   `json:"assigned_by"`      // optional for "assign", defaults to "api"
		DurationSeconds int      `json:"duration_seconds"` // required for "snooze"
		SnoozeReason    string   `json:"snooze_reason"`    // optional for "snooze"
		SnoozedBy       string   `json:"snoozed_by"`       // optional for "snooze", defaults to "api"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.IssueIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "issue_ids is required")
		return
	}
	if len(body.IssueIDs) > 100 {
		writeErr(w, http.StatusBadRequest, "maximum 100 issues per bulk operation")
		return
	}

	ctx := r.Context()
	type result struct {
		IssueID string `json:"issue_id"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}

	var affected int64
	var bulkErr error
	var lifecycleType db.LifecycleEventType
	lifecycleCtx := map[string]interface{}{"trigger": "bulk_api"}

	switch body.Action {
	case "resolve":
		affected, bulkErr = h.DB.BulkResolveIssues(ctx, projectID, body.IssueIDs)
		lifecycleType = db.LifecycleResolved
	case "ignore":
		affected, bulkErr = h.DB.BulkIgnoreIssues(ctx, projectID, body.IssueIDs)
		lifecycleType = db.LifecycleIgnored
	case "unresolve":
		affected, bulkErr = h.DB.BulkUnresolveIssues(ctx, projectID, body.IssueIDs)
		lifecycleType = "" // no lifecycle event for unresolve
	case "assign":
		if body.AssignedTo == "" {
			writeErr(w, http.StatusBadRequest, "assigned_to is required for assign action")
			return
		}
		assignedBy := body.AssignedBy
		if assignedBy == "" {
			assignedBy = "api"
		}
		for _, id := range body.IssueIDs {
			if err := h.DB.AssignIssue(ctx, id, projectID, body.AssignedTo, assignedBy); err != nil {
				h.Log.Error("bulk assign issue", "issue_id", id, "err", err)
			} else {
				affected++
			}
		}
		lifecycleType = db.LifecycleAssigned
		lifecycleCtx["target"] = body.AssignedTo
		lifecycleCtx["assigned_by"] = assignedBy
		// Send Slack DMs for assignments.
		if h.SlackDMs != nil {
			for _, id := range body.IssueIDs {
				h.SlackDMs.NotifyAssignment(ctx, projectID, id, body.AssignedTo, assignedBy)
			}
		}
	case "snooze":
		if body.DurationSeconds <= 0 {
			writeErr(w, http.StatusBadRequest, "duration_seconds is required for snooze action")
			return
		}
		snoozedBy := body.SnoozedBy
		if snoozedBy == "" {
			snoozedBy = "api"
		}
		duration := time.Duration(body.DurationSeconds) * time.Second
		affected, bulkErr = h.DB.BulkSnoozeIssues(ctx, projectID, body.IssueIDs, body.SnoozeReason, snoozedBy, duration)
		lifecycleType = db.LifecycleSnoozed
		lifecycleCtx["reason"] = body.SnoozeReason
		lifecycleCtx["duration_seconds"] = body.DurationSeconds
		lifecycleCtx["snoozed_by"] = snoozedBy
	case "unsnooze":
		affected, bulkErr = h.DB.BulkUnsnoozeIssues(ctx, projectID, body.IssueIDs)
		lifecycleType = db.LifecycleUnsnoozed
	default:
		writeErr(w, http.StatusBadRequest, "action must be 'resolve', 'ignore', 'unresolve', 'assign', 'snooze', or 'unsnooze'")
		return
	}

	if bulkErr != nil {
		h.Log.Error("bulk operation", "action", body.Action, "err", bulkErr)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Record lifecycle events for each affected issue.
	if lifecycleType != "" {
		for _, id := range body.IssueIDs {
			_ = h.DB.InsertLifecycleEvent(ctx, projectID, id, lifecycleType, nil, nil, lifecycleCtx)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"action":   body.Action,
		"affected": affected,
		"total":    len(body.IssueIDs),
	})
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

func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	limit := queryInt(r, "limit", 25)
	releases, err := h.DB.ListReleases(r.Context(), projectID, limit)
	if err != nil {
		h.Log.Error("list releases", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"releases": releases,
	})
}

func (h *Handler) getRelease(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	version := r.PathValue("version")
	if projectID <= 0 || version == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	release, err := h.DB.GetRelease(r.Context(), projectID, version)
	if err != nil {
		writeErr(w, http.StatusNotFound, "release not found")
		return
	}

	writeJSON(w, http.StatusOK, release)
}

func (h *Handler) registerDeploy(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var body struct {
		Version     string `json:"version"`
		Environment string `json:"environment"`
		CommitSHA   string `json:"commit_sha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Version == "" {
		writeErr(w, http.StatusBadRequest, "version is required")
		return
	}

	if err := h.DB.RegisterDeploy(r.Context(), projectID, body.Version, body.Environment, body.CommitSHA); err != nil {
		h.Log.Error("register deploy", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":  "ok",
		"version": body.Version,
	})
}

func (h *Handler) listAlertRules(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	rules, err := h.DB.EnsureDefaultAlertRules(r.Context(), projectID)
	if err != nil {
		h.Log.Error("list alert rules", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

func (h *Handler) getAlertRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	ruleID := r.PathValue("rule_id")
	if projectID <= 0 || ruleID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	rule, err := h.DB.GetAlertRule(r.Context(), projectID, ruleID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "rule not found")
		return
	}

	writeJSON(w, http.StatusOK, rule)
}

func (h *Handler) createAlertRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var rule db.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	rule.ProjectID = projectID

	if rule.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if rule.ConditionType == "" {
		writeErr(w, http.StatusBadRequest, "condition_type is required")
		return
	}

	if err := h.DB.InsertAlertRule(r.Context(), &rule); err != nil {
		h.Log.Error("create alert rule", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

func (h *Handler) updateAlertRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	ruleID := r.PathValue("rule_id")
	if projectID <= 0 || ruleID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	existing, err := h.DB.GetAlertRule(r.Context(), projectID, ruleID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "rule not found")
		return
	}

	var update db.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	existing.Name = update.Name
	existing.Enabled = update.Enabled
	existing.ConditionType = update.ConditionType
	existing.Threshold = update.Threshold
	existing.WindowMinutes = update.WindowMinutes
	existing.LevelFilter = update.LevelFilter
	existing.ActionType = update.ActionType
	existing.ActionTarget = update.ActionTarget

	if err := h.DB.UpdateAlertRule(r.Context(), existing); err != nil {
		h.Log.Error("update alert rule", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	ruleID := r.PathValue("rule_id")
	if projectID <= 0 || ruleID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.DeleteAlertRule(r.Context(), projectID, ruleID); err != nil {
		h.Log.Error("delete alert rule", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listAlertHistory(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	limit := queryInt(r, "limit", 50)
	history, err := h.DB.ListAlertHistory(r.Context(), projectID, limit)
	if err != nil {
		h.Log.Error("list alert history", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"history": history})
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
	_, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
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
	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "issue not found")
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

// resolveHook handles POST /api/hooks/resolve — called by Gas Town (refinery)
// after a fix is merged to mark the faultline issue as resolved.
// Requires HMAC signature verification via X-Hook-Signature-256 header.
func (h *Handler) resolveHook(w http.ResponseWriter, r *http.Request) {
	// Read body for both signature verification and JSON parsing.
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read error")
		return
	}

	// Verify HMAC signature if secret is configured.
	if h.HookSecret != "" {
		sig := r.Header.Get("X-Hook-Signature-256")
		if !verifyHookSignature(rawBody, sig, h.HookSecret) {
			writeErr(w, http.StatusUnauthorized, "invalid or missing signature")
			return
		}
	}

	var body struct {
		ProjectID  int64  `json:"project_id"`
		GroupID    string `json:"group_id"`
		BeadID     string `json:"bead_id"`
		CommitHash string `json:"commit_hash,omitempty"`
		MergedBy   string `json:"merged_by,omitempty"`
		Solution   string `json:"solution,omitempty"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
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

	resolveCtx := map[string]interface{}{
		"trigger":  "resolve_hook",
		"verified": body.CommitHash != "",
	}
	if body.CommitHash != "" {
		resolveCtx["commit_sha"] = body.CommitHash
		// Store commit info on bead record.
		_ = h.DB.UpdateBeadCommitInfo(r.Context(), body.ProjectID, body.GroupID, body.CommitHash, "")
		_ = h.DB.InsertLifecycleEvent(r.Context(), body.ProjectID, body.GroupID, db.LifecycleCommitDetected, &body.BeadID, nil, map[string]interface{}{
			"commit_sha": body.CommitHash,
			"source":     "resolve_hook",
		})
	}
	if body.MergedBy != "" {
		resolveCtx["merged_by"] = body.MergedBy
	}
	if body.Solution != "" {
		resolveCtx["solution"] = body.Solution
	}
	_ = h.DB.InsertLifecycleEvent(r.Context(), body.ProjectID, body.GroupID, db.LifecycleResolved, &body.BeadID, nil, resolveCtx)

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

	p, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil || p == nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	// DSN format: {protocol}://{public_key}@{host}/{project_id}
	dsn := fmt.Sprintf("%s/%d", h.BaseURL, p.ID)
	if h.BaseURL != "" && p.DSNPublicKey != "" {
		if u, err := url.Parse(h.BaseURL); err == nil {
			dsn = fmt.Sprintf("%s://%s@%s/%d", u.Scheme, p.DSNPublicKey, u.Host, p.ID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"project_id": p.ID,
		"public_key": p.DSNPublicKey,
		"rig":        p.Slug,
		"dsn":        dsn,
		"endpoints": map[string]string{
			"envelope": fmt.Sprintf("%s/api/%d/envelope/", h.BaseURL, p.ID),
			"store":    fmt.Sprintf("%s/api/%d/store/", h.BaseURL, p.ID),
		},
	})
}

// getIssueHistory returns the commit-level history of changes to an issue group.
// Uses Dolt's dolt_history system table to show how the issue changed over time.
func (h *Handler) getIssueHistory(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	limit := queryInt(r, "limit", 20)

	snapshots, err := h.DB.IssueGroupHistory(r.Context(), projectID, issueID, limit)
	if err != nil {
		h.Log.Error("issue history", "err", err, "project_id", projectID, "issue_id", issueID)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"snapshots": snapshots,
	})
}

// getIssueAsOf returns the state of an issue at a specific point in time.
// Uses Dolt's AS OF query to read historical data.
func (h *Handler) getIssueAsOf(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	tsParam := r.URL.Query().Get("timestamp")
	if tsParam == "" {
		writeErr(w, http.StatusBadRequest, "timestamp query parameter is required")
		return
	}

	asOf, err := time.Parse(time.RFC3339, tsParam)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid timestamp format, use RFC3339 (e.g. 2024-01-15T10:00:00Z)")
		return
	}

	issue, err := h.DB.IssueGroupAsOf(r.Context(), projectID, issueID, asOf)
	if err != nil {
		h.Log.Error("issue as-of", "err", err, "project_id", projectID, "issue_id", issueID)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if issue == nil {
		writeErr(w, http.StatusNotFound, "issue not found at specified time")
		return
	}

	// Optionally include event count at that time.
	eventCount, err := h.DB.EventCountAsOf(r.Context(), projectID, issueID, asOf)
	if err != nil {
		h.Log.Warn("event count as-of failed", "err", err)
		// Non-fatal — still return the issue.
		eventCount = -1
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"issue":       issue,
		"event_count": eventCount,
		"as_of":       asOf.Format(time.RFC3339),
	})
}

// getLifecycle returns the audit log of lifecycle events for an issue group.
func (h *Handler) getLifecycle(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	if projectID <= 0 || issueID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	entries, err := h.DB.ListLifecycleEvents(r.Context(), projectID, issueID, limit, offset)
	if err != nil {
		h.Log.Error("list lifecycle events", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lifecycle": entries,
	})
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
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// verifyHookSignature checks the HMAC-SHA256 signature of a webhook payload.
func verifyHookSignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
