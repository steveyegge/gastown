// Package dashboard serves the faultline web UI.
package dashboard

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/slackdm"
)

//go:embed static
var staticFS embed.FS

// Handler serves the dashboard UI.
type Handler struct {
	DB            *db.DB
	Log           *slog.Logger
	SlackDMs      *slackdm.Sender  // optional; sends Slack DMs for mentions/assignments
	loginAttempts map[string][]time.Time // IP → recent attempt timestamps
}

// checkLoginRate returns true if the IP has exceeded 5 attempts in the last 15 minutes.
func (h *Handler) checkLoginRate(ip string) bool {
	if h.loginAttempts == nil {
		h.loginAttempts = make(map[string][]time.Time)
	}
	cutoff := time.Now().Add(-15 * time.Minute)
	attempts := h.loginAttempts[ip]
	var recent []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	h.loginAttempts[ip] = recent
	return len(recent) >= 5
}

func (h *Handler) recordLoginAttempt(ip string) {
	if h.loginAttempts == nil {
		h.loginAttempts = make(map[string][]time.Time)
	}
	h.loginAttempts[ip] = append(h.loginAttempts[ip], time.Now())
}

// RegisterRoutes adds dashboard routes to the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static assets.
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Auth routes (no session required).
	mux.HandleFunc("GET /dashboard/login", h.showLogin)
	mux.HandleFunc("POST /dashboard/login", h.doLogin)
	mux.HandleFunc("GET /dashboard/setup", h.showSetup)
	mux.HandleFunc("POST /dashboard/setup", h.doSetup)
	mux.HandleFunc("GET /dashboard/logout", h.doLogout)

	// Shorthand role wrappers for route registration.
	member := func(h http.HandlerFunc) http.HandlerFunc { return requireDashRole("member", h) }
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireDashRole("admin", h) }

	// Protected routes (session required).
	// Viewer: read-only access (all GET pages).
	mux.HandleFunc("GET /dashboard/", h.requireAuth(h.index))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues", h.requireAuth(h.listIssues))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/", h.requireAuth(h.listIssues))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/{issue_id}", h.requireAuth(h.showIssue))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/{issue_id}/", h.requireAuth(h.showIssue))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/events/{event_id}", h.requireAuth(h.showEvent))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/events/{event_id}/", h.requireAuth(h.showEvent))
	mux.HandleFunc("GET /dashboard/partials/projects", h.requireAuth(h.projectsPartial))
	mux.HandleFunc("GET /dashboard/partials/projects/{project_id}/issues", h.requireAuth(h.issuesPartial))
	mux.HandleFunc("GET /dashboard/partials/projects/{project_id}/issues/{issue_id}", h.requireAuth(h.issueDetailPartial))
	mux.HandleFunc("GET /dashboard/live", h.requireAuth(h.liveStream))
	mux.HandleFunc("GET /dashboard/live/", h.requireAuth(h.liveStream))
	mux.HandleFunc("GET /dashboard/live/events", h.requireAuth(h.liveSSE))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/search", h.requireAuth(h.searchIssues))

	// Member: resolve, assign, dispatch, bulk ops, comments, merge, snooze.
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/resolve", h.requireAuth(member(h.resolveIssue)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/dispatch", h.requireAuth(member(h.dispatchIssue)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/bulk", h.requireAuth(member(h.bulkOperateIssues)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/comments", h.requireAuth(member(h.createComment)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/merge", h.requireAuth(member(h.mergeIssueForm)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/unmerge", h.requireAuth(member(h.unmergeIssueForm)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/snooze", h.requireAuth(member(h.snoozeIssueForm)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/unsnooze", h.requireAuth(member(h.unsnoozeIssueForm)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/assign", h.requireAuth(member(h.assignIssueForm)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/unassign", h.requireAuth(member(h.unassignIssueForm)))

	// Admin: settings, teams, integrations.
	mux.HandleFunc("GET /dashboard/projects/{project_id}/settings", h.requireAuth(admin(h.showSettings)))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/settings/", h.requireAuth(admin(h.showSettings)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/settings", h.requireAuth(admin(h.saveSettings)))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/settings/", h.requireAuth(admin(h.saveSettings)))

	// Team routes (admin).
	mux.HandleFunc("GET /dashboard/teams", h.requireAuth(h.listTeamsPage))
	mux.HandleFunc("GET /dashboard/teams/", h.requireAuth(h.listTeamsPage))
	mux.HandleFunc("POST /dashboard/teams/create", h.requireAuth(admin(h.createTeamForm)))
	mux.HandleFunc("GET /dashboard/teams/{team_id}", h.requireAuth(h.showTeam))
	mux.HandleFunc("GET /dashboard/teams/{team_id}/", h.requireAuth(h.showTeam))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/update", h.requireAuth(admin(h.updateTeamForm)))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/delete", h.requireAuth(admin(h.deleteTeamForm)))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/members/add", h.requireAuth(admin(h.addTeamMemberForm)))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/members/{account_id}/remove", h.requireAuth(admin(h.removeTeamMemberForm)))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/projects/link", h.requireAuth(admin(h.linkTeamProjectForm)))
	mux.HandleFunc("POST /dashboard/teams/{team_id}/projects/{project_id}/unlink", h.requireAuth(admin(h.unlinkTeamProjectForm)))

	// Slack account linking (member — per-user operation).
	mux.HandleFunc("GET /dashboard/account/slack", h.requireAuth(h.showSlackLink))
	mux.HandleFunc("POST /dashboard/account/slack", h.requireAuth(member(h.saveSlackLink)))
	mux.HandleFunc("POST /dashboard/account/slack/unlink", h.requireAuth(member(h.unlinkSlack)))

	// Account role management (owner-only).
	owner := func(h http.HandlerFunc) http.HandlerFunc { return requireDashRole("owner", h) }
	mux.HandleFunc("GET /dashboard/accounts", h.requireAuth(owner(h.listAccounts)))
	mux.HandleFunc("GET /dashboard/accounts/", h.requireAuth(owner(h.listAccounts)))
	mux.HandleFunc("POST /dashboard/accounts/update-role", h.requireAuth(owner(h.updateAccountRole)))
}

// redirectToLogin sends the user to the login page. For HTMX requests,
// it uses HX-Redirect so the browser does a full navigation instead of
// swapping the login page into a partial target.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/dashboard/login")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// requireAuth wraps a handler with session authentication.
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		if token == "" {
			redirectToLogin(w, r)
			return
		}
		account, err := h.DB.GetSession(r.Context(), token)
		if err != nil || account == nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
			redirectToLogin(w, r)
			return
		}
		// Store account in context via header (simple approach).
		r.Header.Set("X-Account-Name", account.Name)
		r.Header.Set("X-Account-ID", strconv.FormatInt(account.ID, 10))
		r.Header.Set("X-Account-Role", account.Role)
		// Ensure CSRF token cookie is set for authenticated requests.
		if getCSRFToken(r) == "" {
			csrfToken := generateCSRFToken()
			setCSRFCookie(w, csrfToken)
			r.Header.Set("X-CSRF-Token", csrfToken)
		} else {
			r.Header.Set("X-CSRF-Token", getCSRFToken(r))
		}
		next(w, r)
	}
}

func (h *Handler) currentAccount(r *http.Request) *AccountView {
	name := r.Header.Get("X-Account-Name")
	if name == "" {
		return nil
	}
	return &AccountView{
		Name:      name,
		Role:      r.Header.Get("X-Account-Role"),
		CSRFToken: r.Header.Get("X-CSRF-Token"),
	}
}

// CSRF protection — double-submit cookie pattern.
// On GET, set a csrf_token cookie and pass the token to forms.
// On POST, verify the form token matches the cookie token.

func generateCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		MaxAge:   3600,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func getCSRFToken(r *http.Request) string {
	c, err := r.Cookie("csrf_token")
	if err != nil {
		return ""
	}
	return c.Value
}

func validateCSRF(r *http.Request) bool {
	cookie := getCSRFToken(r)
	form := r.FormValue("csrf_token")
	return cookie != "" && form != "" && cookie == form
}

// Auth handlers

func (h *Handler) showLogin(w http.ResponseWriter, r *http.Request) {
	// Redirect to setup if no accounts exist.
	count, _ := h.DB.AccountCount(r.Context())
	if count == 0 {
		http.Redirect(w, r, "/dashboard/setup", http.StatusSeeOther)
		return
	}
	_ = loginPage("").Render(r.Context(), w)
}

func (h *Handler) doLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	ip := r.RemoteAddr
	if h.checkLoginRate(ip) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = loginPage("Too many login attempts. Please wait 15 minutes.").Render(r.Context(), w)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	account, err := h.DB.Authenticate(r.Context(), email, password)
	if err != nil {
		h.recordLoginAttempt(ip)
		_ = loginPage("Invalid email or password.").Render(r.Context(), w)
		return
	}

	token, err := h.DB.CreateSession(r.Context(), account.ID)
	if err != nil {
		h.Log.Error("create session", "err", err)
		_ = loginPage("Internal error.").Render(r.Context(), w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60, // 7 days
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

func (h *Handler) showSetup(w http.ResponseWriter, r *http.Request) {
	count, _ := h.DB.AccountCount(r.Context())
	if count > 0 {
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
		return
	}
	_ = setupPage("").Render(r.Context(), w)
}

func (h *Handler) doSetup(w http.ResponseWriter, r *http.Request) {
	count, _ := h.DB.AccountCount(r.Context())
	if count > 0 {
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if name == "" || email == "" || len(password) < 8 {
		_ = setupPage("Name, email required. Password must be 8+ characters.").Render(r.Context(), w)
		return
	}

	account, err := h.DB.CreateAccount(r.Context(), email, name, password, "owner")
	if err != nil {
		h.Log.Error("create account", "err", err)
		_ = setupPage("Could not create account.").Render(r.Context(), w)
		return
	}

	token, err := h.DB.CreateSession(r.Context(), account.ID)
	if err != nil {
		h.Log.Error("create session", "err", err)
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

func (h *Handler) doLogout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		_ = h.DB.DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// Dashboard handlers

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	views := h.buildProjectViews(r)
	_ = projectsPage(account, views).Render(r.Context(), w)
}

func (h *Handler) listIssues(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	projectID := pathInt64(r, "project_id")
	status := r.URL.Query().Get("status")
	// Default to active (unresolved + regressed) issues when no status filter is specified.
	if !r.URL.Query().Has("status") {
		status = "active"
	}
	sortBy := r.URL.Query().Get("sort")
	page := queryInt(r, "page")
	if page < 1 {
		page = 1
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Get platform info for this project.
	stats, _ := h.DB.GetProjectStats(r.Context(), projectID)
	platform := ""
	if stats != nil {
		platform = stats.PrimaryPlatform
	}

	env := r.URL.Query().Get("env")
	platformFilter := r.URL.Query().Get("platform")
	assigneeFilter := r.URL.Query().Get("assignee")

	if sortBy == "" {
		sortBy = "severity" // default: severity first
	}
	params := db.IssueListParams{
		ProjectID:   projectID,
		Status:      status,
		Environment: env,
		Platform:    platformFilter,
		Assignee:    assigneeFilter,
		Sort:        sortBy,
		Limit:       25,
		Offset:      (page - 1) * 25,
	}

	issues, total, err := h.DB.ListIssueGroups(r.Context(), params)
	if err != nil {
		h.Log.Error("list issues", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Batch-fetch latest lifecycle stage for all issues on this page.
	groupIDs := make([]string, len(issues))
	for i, ig := range issues {
		groupIDs[i] = ig.ID
	}
	lifecycleMap, _ := h.DB.LatestLifecycleByGroups(r.Context(), projectID, groupIDs)

	// Batch-fetch assignments for all issues on this page.
	assignmentMap, _ := h.DB.AssignmentsForIssues(r.Context(), projectID, groupIDs)

	var views []IssueView
	for _, ig := range issues {
		iv := IssueView{
			ID:         ig.ID,
			ProjectID:  ig.ProjectID,
			Title:      ig.Title,
			Culprit:    ig.Culprit,
			Level:      ig.Level,
			Platform:   ig.Platform,
			Status:     ig.Status,
			FirstSeen:  ig.FirstSeen,
			LastSeen:   ig.LastSeen,
			EventCount: ig.EventCount,
		}
		if a, ok := assignmentMap[ig.ID]; ok {
			iv.AssignedTo = a.AssignedTo
		}
		if lc, ok := lifecycleMap[ig.ID]; ok {
			iv.LifecycleStage = string(lc.EventType)
			iv.LifecycleTime = lc.Timestamp
			// Extract the actor: prefer target from context, fall back to rig.
			if lc.Context != nil {
				var ctx map[string]interface{}
				if json.Unmarshal(lc.Context, &ctx) == nil {
					if t, ok := ctx["target"].(string); ok && t != "" {
						iv.LifecycleActor = t
					}
				}
			}
			if iv.LifecycleActor == "" && lc.Rig != nil {
				iv.LifecycleActor = *lc.Rig
			}
		}
		views = append(views, iv)
	}

	// Get available environments and platforms for the filter tabs.
	envs, _ := h.DB.ProjectEnvironments(r.Context(), projectID)
	plats, _ := h.DB.ProjectPlatforms(r.Context(), projectID)

	// Load accounts for the bulk assign dropdown.
	accounts, _ := h.DB.ListAccounts(r.Context())

	_ = seismographPage(account, views, total, project.Name, platform, projectID, status, sortBy, page, env, envs, platformFilter, plats, accounts).Render(r.Context(), w)
}

func (h *Handler) showIssue(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}

	project, _ := h.DB.GetProject(r.Context(), projectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	events, err := h.DB.ListEventsByGroup(r.Context(), projectID, issueID, 20, 0)
	if err != nil {
		h.Log.Error("list events", "err", err)
	}

	// Fetch lifecycle audit log.
	lifecycle, _ := h.DB.ListLifecycleEvents(r.Context(), projectID, issueID, 50, 0)

	// Fetch current assignment.
	assignment, _ := h.DB.GetIssueAssignment(r.Context(), issueID, projectID)

	iv := IssueView{
		ID: issue.ID, ProjectID: issue.ProjectID,
		Title: issue.Title, Culprit: issue.Culprit, Level: issue.Level,
		Status: issue.Status, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen,
		EventCount: issue.EventCount, BeadID: issue.BeadID,
		RootCause: issue.RootCause, FixExplanation: issue.FixExplanation, FixCommit: issue.FixCommit,
		MergedInto: issue.MergedInto,
		SnoozedUntil: issue.SnoozedUntil, SnoozeReason: issue.SnoozeReason, SnoozedBy: issue.SnoozedBy,
	}
	if assignment != nil {
		iv.AssignedTo = assignment.AssignedTo
	}

	// If merged, fetch target issue title.
	if issue.MergedInto != "" {
		target, err := h.DB.GetIssueGroup(r.Context(), projectID, issue.MergedInto)
		if err == nil {
			iv.MergedIntoTitle = target.Title
		}
	}

	// Fetch issues merged into this one.
	mergedChildren, _ := h.DB.ListMergedInto(r.Context(), projectID, issueID)
	for _, mc := range mergedChildren {
		iv.MergedChildren = append(iv.MergedChildren, MergedIssueView{
			ID:         mc.ID,
			Title:      mc.Title,
			EventCount: mc.EventCount,
		})
	}

	// Enrich with latest event details (platform, exception info).
	if len(events) > 0 {
		latest := events[0] // already sorted DESC
		iv.Platform = latest.Platform
		iv.ExceptionType = latest.ExceptionType
		iv.ExceptionValue = latest.Message
	}

	// Extract resolution explanation from lifecycle events.
	for _, lc := range lifecycle {
		if string(lc.EventType) == "resolved" {
			iv.Resolution = resolveExplanation(lc)
			break
		}
	}

	var evs []EventView
	for _, e := range events {
		evs = append(evs, EventView{
			EventID: e.EventID, Timestamp: e.Timestamp,
			Platform: e.Platform, Level: e.Level, Message: e.Message,
			Environment: e.Environment, Release: e.Release,
		})
	}

	var lcViews []LifecycleView
	for _, lc := range lifecycle {
		lv := LifecycleView{
			EventType: string(lc.EventType),
			BeadID:    ptrStr(lc.BeadID),
			Rig:       ptrStr(lc.Rig),
			Context:   string(lc.Context),
			Timestamp: lc.Timestamp,
		}
		// Extract fields from context.
		if lc.Context != nil {
			var ctx map[string]interface{}
			if json.Unmarshal(lc.Context, &ctx) == nil {
				if t, ok := ctx["target"].(string); ok {
					lv.Target = t
				}
				if a, ok := ctx["assignee"].(string); ok {
					lv.Assignee = a
				}
				if a, ok := ctx["resolved_by"].(string); ok {
					lv.Assignee = a
				}
				if s, ok := ctx["solution"].(string); ok {
					lv.Solution = s
				}
				if s, ok := ctx["close_reason"].(string); ok && lv.Solution == "" {
					lv.Solution = s
				}
			}
		}
		lcViews = append(lcViews, lv)
	}

	// Load accounts for the assign dropdown.
	accounts, _ := h.DB.ListAccounts(r.Context())

	_ = faultReportPage(account, iv, evs, lcViews, projectName, projectID, accounts).Render(r.Context(), w)
}

func resolveExplanation(lc db.LifecycleEntry) string {
	if lc.Context == nil {
		return "Issue went quiet — no new events after the fix was deployed."
	}
	var ctx map[string]interface{}
	if err := json.Unmarshal(lc.Context, &ctx); err != nil {
		return "Issue resolved."
	}
	trigger, _ := ctx["trigger"].(string)
	switch trigger {
	case "manual":
		explanation, _ := ctx["explanation"].(string)
		solution, _ := ctx["solution"].(string)
		result := ""
		if explanation != "" {
			result = "Cause: " + explanation
		}
		if solution != "" {
			if result != "" {
				result += "\n"
			}
			result += "Fix: " + solution
		}
		if result == "" {
			return "Manually stabilized."
		}
		return result
	case "auto-resolve-quiet":
		quiet, _ := ctx["quiet"].(string)
		if quiet != "" {
			return "Auto-resolved: no new errors for " + quiet + " after fix was deployed."
		}
		return "Auto-resolved: error stopped recurring after fix was deployed."
	default:
		quietPeriod, _ := ctx["quiet_period"].(string)
		if quietPeriod != "" {
			return "Fix confirmed: bead closed and no new errors for " + quietPeriod + "."
		}
		return "Fix confirmed: bead was closed and error stopped recurring."
	}
}

func (h *Handler) dispatchIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	action := r.FormValue("action")
	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}

	// Look up the rig for this project from the auth registry.
	project, _ := h.DB.GetProject(r.Context(), projectID)
	rig := ""
	if project != nil {
		rig = project.Slug
	}
	if rig == "" {
		rig = "faultline" // fallback
	}

	beadID := issue.BeadID

	switch action {
	case "polecat":
		// File a bead if none exists, then sling to polecat.
		if beadID == "" {
			cmd := exec.Command("bd", "new", "--type", "bug", "--rig", rig, issue.Title) //nolint:gosec
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			if err := cmd.Run(); err != nil {
				h.Log.Error("dispatch: bd new failed", "err", err)
				http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
				return
			}
			beadID = extractBeadID(strings.TrimSpace(stdout.String()))
			if beadID != "" {
				_ = h.DB.InsertBead(r.Context(), issueID, projectID, beadID, rig)
			}
		}
		if beadID != "" {
			cmd := exec.Command("gt", "sling", beadID, rig, "--force") //nolint:gosec
			if err := cmd.Run(); err != nil {
				h.Log.Error("dispatch: sling failed", "bead_id", beadID, "err", err)
			} else {
				_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleDispatched, &beadID, &rig, map[string]interface{}{
					"target":  rig + "/polecats",
					"method":  "dashboard",
					"trigger": "manual dispatch",
				})
			}
		}

	case "crew":
		msg := fmt.Sprintf("Dashboard dispatch: %s needs investigation (project %d, group %s). Please review and decide next steps.",
			issue.Title, projectID, issueID[:12])
		cmd := exec.Command("gt", "nudge", rig+"/crew", msg) //nolint:gosec
		if err := cmd.Run(); err != nil {
			// Try witness, then mayor.
			cmd2 := exec.Command("gt", "nudge", rig+"/witness", msg) //nolint:gosec
			if cmd2.Run() != nil {
				cmd3 := exec.Command("gt", "nudge", "mayor", msg) //nolint:gosec
				_ = cmd3.Run()
			}
		}
		_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleNotified, nil, &rig, map[string]interface{}{
			"target":  rig + "/crew",
			"method":  "dashboard",
			"trigger": "manual notify",
		})
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

// extractBeadID pulls a bead ID from bd new output (e.g. "fl-9dg" from "✓ Created ... fl-9dg — title").
func extractBeadID(output string) string {
	if idx := strings.Index(output, ": "); idx >= 0 {
		rest := output[idx+2:]
		if dashIdx := strings.Index(rest, " —"); dashIdx > 0 {
			candidate := strings.TrimSpace(rest[:dashIdx])
			if len(candidate) > 0 && len(candidate) < 30 {
				return candidate
			}
		}
		fields := strings.Fields(rest)
		if len(fields) > 0 && len(fields[0]) < 30 {
			return fields[0]
		}
	}
	return ""
}

func timeAgoStr(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *Handler) resolveIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	explanation := strings.TrimSpace(r.FormValue("explanation"))
	solution := strings.TrimSpace(r.FormValue("solution"))
	fixCommit := strings.TrimSpace(r.FormValue("fix_commit"))

	if err := h.DB.ResolveIssueGroup(r.Context(), projectID, issueID); err != nil {
		h.Log.Error("resolve issue", "err", err)
	}

	// Persist root cause and fix explanation on the issue itself.
	if err := h.DB.UpdateIssueResolution(r.Context(), projectID, issueID, explanation, solution, fixCommit); err != nil {
		h.Log.Error("update issue resolution", "err", err)
	}

	// Record the resolution with explanation in the lifecycle audit log.
	_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleResolved, nil, nil, map[string]interface{}{
		"trigger":     "manual",
		"explanation": explanation,
		"solution":    solution,
	})

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

func (h *Handler) snoozeIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	durationHours, _ := strconv.Atoi(r.FormValue("duration_hours"))
	if durationHours <= 0 {
		durationHours = 24
	}

	account := h.currentAccount(r)
	snoozedBy := "dashboard"
	if account != nil {
		snoozedBy = account.Name
	}

	duration := time.Duration(durationHours) * time.Hour
	if err := h.DB.SnoozeIssueGroup(r.Context(), projectID, issueID, reason, snoozedBy, duration); err != nil {
		h.Log.Error("snooze issue", "err", err)
	}

	_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleSnoozed, nil, nil, map[string]interface{}{
		"trigger":        "manual",
		"reason":         reason,
		"duration_hours": durationHours,
		"snoozed_by":     snoozedBy,
	})

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

func (h *Handler) unsnoozeIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.UnsnoozeIssueGroup(r.Context(), projectID, issueID); err != nil {
		h.Log.Error("unsnooze issue", "err", err)
	}

	account := h.currentAccount(r)
	actor := "dashboard"
	if account != nil {
		actor = account.Name
	}

	_ = h.DB.InsertLifecycleEvent(r.Context(), projectID, issueID, db.LifecycleUnsnoozed, nil, nil, map[string]interface{}{
		"trigger":      "manual",
		"unsnoozed_by": actor,
	})

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

func (h *Handler) assignIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	assignedTo := strings.TrimSpace(r.FormValue("assigned_to"))
	if assignedTo == "" {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
		return
	}

	account := h.currentAccount(r)
	assignedBy := "dashboard"
	if account != nil {
		assignedBy = account.Name
	}

	ctx := r.Context()
	if err := h.DB.AssignIssue(ctx, issueID, projectID, assignedTo, assignedBy); err != nil {
		h.Log.Error("assign issue", "err", err)
	}

	_ = h.DB.InsertLifecycleEvent(ctx, projectID, issueID, db.LifecycleAssigned, nil, nil, map[string]interface{}{
		"trigger":  "manual",
		"assignee": assignedTo,
		"actor":    assignedBy,
	})

	if h.SlackDMs != nil {
		h.SlackDMs.NotifyAssignment(ctx, projectID, issueID, assignedTo, assignedBy)
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

func (h *Handler) unassignIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.UnassignIssue(r.Context(), issueID, projectID); err != nil {
		h.Log.Error("unassign issue", "err", err)
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}

func (h *Handler) bulkOperateIssues(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	action := r.FormValue("action")
	issueIDs := r.Form["issue_ids"]
	assignedTo := strings.TrimSpace(r.FormValue("assigned_to"))
	account := h.currentAccount(r)

	if len(issueIDs) == 0 || action == "" {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues", projectID), http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	assignedBy := "dashboard"
	if account != nil {
		assignedBy = account.Name
	}

	var lifecycleType db.LifecycleEventType
	lifecycleCtx := map[string]interface{}{"trigger": "bulk_dashboard", "actor": assignedBy}

	switch action {
	case "resolve":
		_, _ = h.DB.BulkResolveIssues(ctx, projectID, issueIDs)
		lifecycleType = db.LifecycleResolved
	case "ignore":
		_, _ = h.DB.BulkIgnoreIssues(ctx, projectID, issueIDs)
		lifecycleType = db.LifecycleIgnored
	case "unresolve":
		_, _ = h.DB.BulkUnresolveIssues(ctx, projectID, issueIDs)
	case "assign":
		if assignedTo == "" {
			http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues", projectID), http.StatusSeeOther)
			return
		}
		for _, id := range issueIDs {
			_ = h.DB.AssignIssue(ctx, id, projectID, assignedTo, assignedBy)
		}
		lifecycleType = db.LifecycleAssigned
		lifecycleCtx["target"] = assignedTo
		// Send Slack DMs for assignments.
		if h.SlackDMs != nil {
			for _, id := range issueIDs {
				h.SlackDMs.NotifyAssignment(ctx, projectID, id, assignedTo, assignedBy)
			}
		}
	case "snooze":
		durationHours, _ := strconv.Atoi(r.FormValue("snooze_hours"))
		if durationHours <= 0 {
			durationHours = 24
		}
		reason := strings.TrimSpace(r.FormValue("snooze_reason"))
		duration := time.Duration(durationHours) * time.Hour
		_, _ = h.DB.BulkSnoozeIssues(ctx, projectID, issueIDs, reason, assignedBy, duration)
		lifecycleType = db.LifecycleSnoozed
		lifecycleCtx["reason"] = reason
		lifecycleCtx["duration_hours"] = durationHours
	case "unsnooze":
		_, _ = h.DB.BulkUnsnoozeIssues(ctx, projectID, issueIDs)
		lifecycleType = db.LifecycleUnsnoozed
	}

	if lifecycleType != "" {
		for _, id := range issueIDs {
			_ = h.DB.InsertLifecycleEvent(ctx, projectID, id, lifecycleType, nil, nil, lifecycleCtx)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues", projectID), http.StatusSeeOther)
}

func (h *Handler) showSettings(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	projectID := pathInt64(r, "project_id")

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	cfg := project.Config
	if cfg == nil {
		cfg = &db.ProjectConfig{}
	}

	// Build Sentry-compatible DSN: scheme://public_key@host/project_id
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	dsn := fmt.Sprintf("%s://%s@%s/%d", scheme, project.DSNPublicKey, r.Host, project.ID)

	sv := SettingsView{
		ProjectID:        project.ID,
		ProjectName:      project.Name,
		DSN:              dsn,
		Description:      cfg.Description,
		URL:              cfg.URL,
		DeploymentType:   string(cfg.DeploymentType),
		Components:       strings.Join(cfg.Components, ", "),
		Environments:     strings.Join(cfg.Environments, ", "),
		WebhookURL:       cfg.WebhookURL,
		WebhookType:      string(cfg.WebhookType),
		WebhookTemplates: cfg.WebhookTemplates,
	}

	// Load alert rules (auto-creates defaults if none exist).
	if rules, err := h.DB.EnsureDefaultAlertRules(r.Context(), project.ID); err == nil {
		sv.AlertRules = rules
	}
	// Load recent alert history.
	if history, err := h.DB.ListAlertHistory(r.Context(), project.ID, 20); err == nil {
		sv.AlertHistory = history
	}

	_ = settingsPage(account, sv, "").Render(r.Context(), w)
}

func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	cfg := project.Config
	if cfg == nil {
		cfg = &db.ProjectConfig{}
	}

	cfg.Description = strings.TrimSpace(r.FormValue("description"))
	cfg.URL = strings.TrimSpace(r.FormValue("url"))

	dt := strings.TrimSpace(r.FormValue("deployment_type"))
	switch dt {
	case "local", "remote", "hosted":
		cfg.DeploymentType = db.DeploymentType(dt)
	default:
		cfg.DeploymentType = ""
	}

	// Parse comma-separated lists.
	cfg.Components = splitTrim(r.FormValue("components"))
	cfg.Environments = splitTrim(r.FormValue("environments"))

	cfg.WebhookURL = strings.TrimSpace(r.FormValue("webhook_url"))
	wt := strings.TrimSpace(r.FormValue("webhook_type"))
	switch wt {
	case "slack", "discord", "generic":
		cfg.WebhookType = db.WebhookType(wt)
	default:
		cfg.WebhookType = db.WebhookSlack
	}

	if err := h.DB.UpdateProjectConfig(r.Context(), projectID, cfg); err != nil {
		h.Log.Error("save settings", "err", err)
		account := h.currentAccount(r)
		sv := SettingsView{
			ProjectID:      project.ID,
			ProjectName:    project.Name,
			Description:    cfg.Description,
			URL:            cfg.URL,
			DeploymentType: string(cfg.DeploymentType),
			Components:     strings.Join(cfg.Components, ", "),
			Environments:   strings.Join(cfg.Environments, ", "),
			WebhookURL:     cfg.WebhookURL,
			WebhookType:    string(cfg.WebhookType),
		}
		_ = settingsPage(account, sv, "Failed to save settings.").Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues", projectID), http.StatusSeeOther)
}

func splitTrim(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (h *Handler) currentAccountID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.Header.Get("X-Account-ID"), 10, 64)
	return id
}

func (h *Handler) showSlackLink(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	accountID := h.currentAccountID(r)
	mapping, _ := h.DB.GetSlackUserMapping(r.Context(), accountID)
	_ = slackLinkPage(account, mapping).Render(r.Context(), w)
}

func (h *Handler) saveSlackLink(w http.ResponseWriter, r *http.Request) {
	accountID := h.currentAccountID(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	slackUserID := strings.TrimSpace(r.FormValue("slack_user_id"))
	slackTeamID := strings.TrimSpace(r.FormValue("slack_team_id"))
	if slackUserID == "" {
		http.Redirect(w, r, "/dashboard/account/slack", http.StatusSeeOther)
		return
	}

	if err := h.DB.UpsertSlackUserMapping(r.Context(), accountID, slackUserID, slackTeamID); err != nil {
		h.Log.Error("save slack link", "err", err)
		http.Error(w, "failed to save", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/dashboard/account/slack", http.StatusSeeOther)
}

func (h *Handler) unlinkSlack(w http.ResponseWriter, r *http.Request) {
	accountID := h.currentAccountID(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.DeleteSlackUserMapping(r.Context(), accountID); err != nil {
		h.Log.Error("unlink slack", "err", err)
	}
	http.Redirect(w, r, "/dashboard/account/slack", http.StatusSeeOther)
}

func (h *Handler) showEvent(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	projectID := pathInt64(r, "project_id")
	eventID := r.PathValue("event_id")

	event, err := h.DB.GetEvent(r.Context(), projectID, eventID)
	if err != nil {
		http.Error(w, "event not found", http.StatusNotFound)
		return
	}

	// Pretty-print raw JSON.
	var pretty json.RawMessage
	if err := json.Unmarshal(event.RawJSON, &pretty); err == nil {
		if formatted, err := json.MarshalIndent(pretty, "", "  "); err == nil {
			pretty = formatted
		}
	}

	ev := EventView{
		EventID: event.EventID, Timestamp: event.Timestamp,
		Platform: event.Platform, Level: event.Level,
		Message: event.Message, RawJSON: string(pretty),
	}

	_ = coreSamplePage(account, ev, projectID, event.GroupID).Render(r.Context(), w)
}

func (h *Handler) projectsPartial(w http.ResponseWriter, r *http.Request) {
	views := h.buildProjectViews(r)
	_ = projectGridPartial(views).Render(r.Context(), w)
}

func (h *Handler) issuesPartial(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "severity"
	}
	page := queryInt(r, "page")
	if page < 1 {
		page = 1
	}

	env := r.URL.Query().Get("env")
	platformFilter := r.URL.Query().Get("platform")
	params := db.IssueListParams{
		ProjectID:   projectID,
		Status:      status,
		Environment: env,
		Platform:    platformFilter,
		Sort:        sortBy,
		Limit:       25,
		Offset:      (page - 1) * 25,
	}
	issues, total, err := h.DB.ListIssueGroups(r.Context(), params)
	if err != nil {
		return
	}

	groupIDs := make([]string, len(issues))
	for i, ig := range issues {
		groupIDs[i] = ig.ID
	}
	lifecycleMap, _ := h.DB.LatestLifecycleByGroups(r.Context(), projectID, groupIDs)
	assignmentMap, _ := h.DB.AssignmentsForIssues(r.Context(), projectID, groupIDs)

	var views []IssueView
	for _, ig := range issues {
		iv := IssueView{
			ID: ig.ID, ProjectID: ig.ProjectID, Title: ig.Title, Culprit: ig.Culprit,
			Level: ig.Level, Platform: ig.Platform, Status: ig.Status, FirstSeen: ig.FirstSeen, LastSeen: ig.LastSeen,
			EventCount: ig.EventCount,
		}
		if a, ok := assignmentMap[ig.ID]; ok {
			iv.AssignedTo = a.AssignedTo
		}
		if rel, err := h.DB.FirstReleaseForIssue(r.Context(), projectID, ig.ID); err == nil {
			iv.FirstRelease = rel
		}
		if lc, ok := lifecycleMap[ig.ID]; ok {
			iv.LifecycleStage = string(lc.EventType)
			iv.LifecycleTime = lc.Timestamp
			if lc.Context != nil {
				var ctx map[string]interface{}
				if json.Unmarshal(lc.Context, &ctx) == nil {
					if t, ok := ctx["target"].(string); ok && t != "" {
						iv.LifecycleActor = t
					}
				}
			}
			if iv.LifecycleActor == "" && lc.Rig != nil {
				iv.LifecycleActor = *lc.Rig
			}
		}
		views = append(views, iv)
	}

	account := h.currentAccount(r)
	accounts, _ := h.DB.ListAccounts(r.Context())
	_ = issueListPartial(views, total, projectID, status, sortBy, page, account, accounts).Render(r.Context(), w)
}

func (h *Handler) issueDetailPartial(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	issue, err := h.DB.GetIssueGroup(r.Context(), projectID, issueID)
	if err != nil {
		return
	}
	project, _ := h.DB.GetProject(r.Context(), projectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	events, _ := h.DB.ListEventsByGroup(r.Context(), projectID, issueID, 20, 0)
	lifecycle, _ := h.DB.ListLifecycleEvents(r.Context(), projectID, issueID, 50, 0)

	assignment, _ := h.DB.GetIssueAssignment(r.Context(), issueID, projectID)
	iv := IssueView{
		ID: issue.ID, ProjectID: issue.ProjectID,
		Title: issue.Title, Culprit: issue.Culprit, Level: issue.Level,
		Status: issue.Status, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen,
		EventCount: issue.EventCount, BeadID: issue.BeadID,
		RootCause: issue.RootCause, FixExplanation: issue.FixExplanation, FixCommit: issue.FixCommit,
		MergedInto: issue.MergedInto,
		SnoozedUntil: issue.SnoozedUntil, SnoozeReason: issue.SnoozeReason, SnoozedBy: issue.SnoozedBy,
	}
	if assignment != nil {
		iv.AssignedTo = assignment.AssignedTo
	}
	if issue.MergedInto != "" {
		if target, err := h.DB.GetIssueGroup(r.Context(), projectID, issue.MergedInto); err == nil {
			iv.MergedIntoTitle = target.Title
		}
	}
	mergedChildren, _ := h.DB.ListMergedInto(r.Context(), projectID, issueID)
	for _, mc := range mergedChildren {
		iv.MergedChildren = append(iv.MergedChildren, MergedIssueView{
			ID: mc.ID, Title: mc.Title, EventCount: mc.EventCount,
		})
	}
	if len(events) > 0 {
		iv.Platform = events[0].Platform
		iv.ExceptionType = events[0].ExceptionType
		iv.ExceptionValue = events[0].Message
	}
	if rel, err := h.DB.FirstReleaseForIssue(r.Context(), projectID, issueID); err == nil {
		iv.FirstRelease = rel
	}
	if rel, err := h.DB.LastReleaseForIssue(r.Context(), projectID, issueID); err == nil {
		iv.LastRelease = rel
	}
	for _, lc := range lifecycle {
		if string(lc.EventType) == "resolved" {
			iv.Resolution = resolveExplanation(lc)
			break
		}
	}

	var evs []EventView
	for _, e := range events {
		evs = append(evs, EventView{
			EventID: e.EventID, Timestamp: e.Timestamp,
			Platform: e.Platform, Level: e.Level, Message: e.Message,
			Environment: e.Environment, Release: e.Release,
		})
	}
	var lcViews []LifecycleView
	for _, lc := range lifecycle {
		lv := LifecycleView{
			EventType: string(lc.EventType), BeadID: ptrStr(lc.BeadID),
			Rig: ptrStr(lc.Rig), Context: string(lc.Context), Timestamp: lc.Timestamp,
		}
		if lc.Context != nil {
			var ctx map[string]interface{}
			if json.Unmarshal(lc.Context, &ctx) == nil {
				if t, ok := ctx["target"].(string); ok {
					lv.Target = t
				}
				if a, ok := ctx["assignee"].(string); ok {
					lv.Assignee = a
				}
				if a, ok := ctx["resolved_by"].(string); ok {
					lv.Assignee = a
				}
				if s, ok := ctx["solution"].(string); ok {
					lv.Solution = s
				}
				if s, ok := ctx["close_reason"].(string); ok && lv.Solution == "" {
					lv.Solution = s
				}
			}
		}
		lcViews = append(lcViews, lv)
	}

	csrfToken := r.Header.Get("X-CSRF-Token")
	accounts, _ := h.DB.ListAccounts(r.Context())
	_ = faultReportPartial(iv, evs, lcViews, projectName, projectID, csrfToken, accounts).Render(r.Context(), w)
}

// buildProjectViews creates ProjectView list (shared by index and partial).
func (h *Handler) buildProjectViews(r *http.Request) []ProjectView {
	projects, err := h.DB.ListProjects(r.Context())
	if err != nil {
		return nil
	}
	var views []ProjectView
	for _, p := range projects {
		stats, _ := h.DB.GetProjectStats(r.Context(), p.ID)
		desc := p.Slug
		if p.Config != nil && p.Config.Description != "" {
			desc = p.Config.Description
		}
		pv := ProjectView{
			ID:          p.ID,
			Name:        p.Name,
			Description: desc,
		}
		if p.Config != nil {
			pv.Components = p.Config.Components
			pv.Environments = p.Config.Environments
			pv.DeploymentType = string(p.Config.DeploymentType)
			pv.IsRemote = p.Config.DeploymentType == db.DeployRemote
			pv.URL = p.Config.URL
		}
		if stats != nil {
			pv.TotalIssues = stats.TotalIssues
			pv.UnresolvedIssues = stats.UnresolvedIssues
			pv.TotalEvents = stats.TotalEvents
			pv.UnbeadedCount = stats.UnbeadedCount
			pv.Platform = stats.PrimaryPlatform
			pv.Platforms = stats.Platforms
			if stats.LastSeen != nil {
				pv.LastSeen = timeAgoStr(*stats.LastSeen)
				if !pv.IsRemote {
					pv.Running = time.Since(*stats.LastSeen) < 24*time.Hour
				}
			}
			monitored := stats.TotalEvents > 0 || stats.TotalIssues > 0
			switch {
			case !monitored:
				pv.Status = StatusUnmonitored
			case stats.UnresolvedIssues == 0:
				pv.Status = StatusGreen
			case stats.UnbeadedCount > 0:
				pv.Status = StatusRed
			default:
				pv.Status = StatusYellow
			}
		} else {
			pv.Status = StatusUnmonitored
		}
		// Attach latest health check and uptime if available.
		hc, err := h.DB.LatestHealthCheck(r.Context(), p.ID)
		if err == nil && hc != nil {
			pv.HealthUp = &hc.Up
			pv.HealthResponseMS = hc.ResponseMS
			pv.HealthCheckedAgo = timeAgoStr(hc.CheckedAt)
		}
		// Sparkline: hourly event counts for last 24h.
		sparkline, _ := h.DB.HourlyEventCounts(r.Context(), p.ID, 24)
		hasData := false
		for _, c := range sparkline {
			if c > 0 {
				hasData = true
				break
			}
		}
		if hasData {
			pv.Sparkline = sparkline
		}

		// Latest release badge for project card.
		if rel, err := h.DB.LatestRelease(r.Context(), p.ID); err == nil {
			pv.LatestRelease = rel.Version
		}

		uptime, total, _ := h.DB.UptimeSince(r.Context(), p.ID, time.Now().Add(-24*time.Hour))
		if total > 0 {
			pv.Uptime24h = fmt.Sprintf("%.1f%%", uptime)
		}

		// Hide projects with no activity and no health checks — they're just
		// registered placeholders that clutter the grid.
		if pv.Status == StatusUnmonitored && pv.HealthUp == nil && pv.Uptime24h == "" {
			continue
		}
		views = append(views, pv)
	}
	return views
}

func (h *Handler) liveStream(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	_ = liveStreamPage(account).Render(r.Context(), w)
}

func (h *Handler) liveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Poll for new events every 2 seconds.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastSeen time.Time
	// Start from 1 minute ago to show some recent events on connect.
	lastSeen = time.Now().UTC().Add(-1 * time.Minute)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			events, err := h.DB.RecentEvents(r.Context(), lastSeen, 20)
			if err != nil {
				continue
			}
			for _, e := range events {
				// Get project name.
				projName := fmt.Sprintf("project-%d", e.ProjectID)
				if p, err := h.DB.GetProject(r.Context(), e.ProjectID); err == nil {
					projName = p.Name
				}

				data := fmt.Sprintf(`{"event_id":"%s","project":"%s","project_id":%d,"level":"%s","message":"%s","platform":"%s","timestamp":"%s"}`,
					e.EventID, projName, e.ProjectID, e.Level,
					strings.ReplaceAll(e.Message, `"`, `\"`),
					e.Platform, e.Timestamp.Format(time.RFC3339),
				)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				if e.Timestamp.After(lastSeen) {
					lastSeen = e.Timestamp
				}
			}
		}
	}
}

// Team handlers

func (h *Handler) listTeamsPage(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)

	teams, err := h.DB.ListTeams(r.Context())
	if err != nil {
		h.Log.Error("list teams", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var views []TeamView
	for _, t := range teams {
		members, _ := h.DB.ListTeamMembers(r.Context(), t.ID)
		projects, _ := h.DB.ListTeamProjects(r.Context(), t.ID)
		views = append(views, TeamView{
			ID:           t.ID,
			Name:         t.Name,
			Slug:         t.Slug,
			MemberCount:  len(members),
			ProjectCount: len(projects),
		})
	}

	_ = teamsListPage(account, views).Render(r.Context(), w)
}

func (h *Handler) createTeamForm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	if name == "" || slug == "" {
		http.Redirect(w, r, "/dashboard/teams", http.StatusSeeOther)
		return
	}

	_, err := h.DB.CreateTeam(r.Context(), name, slug)
	if err != nil {
		h.Log.Error("create team", "err", err)
	}
	http.Redirect(w, r, "/dashboard/teams", http.StatusSeeOther)
}

func (h *Handler) showTeam(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	teamID := pathInt64(r, "team_id")

	team, err := h.DB.GetTeam(r.Context(), teamID)
	if err != nil {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}

	members, _ := h.DB.ListTeamMembers(r.Context(), teamID)
	teamProjects, _ := h.DB.ListTeamProjects(r.Context(), teamID)
	accounts, _ := h.DB.ListAccounts(r.Context())
	allProjects, _ := h.DB.ListProjects(r.Context())

	// Build member views with account names.
	accountMap := make(map[int64]db.Account)
	for _, a := range accounts {
		accountMap[a.ID] = a
	}

	var memberViews []MemberView
	for _, m := range members {
		mv := MemberView{
			AccountID: m.AccountID,
			Role:      m.Role,
		}
		if a, ok := accountMap[m.AccountID]; ok {
			mv.Name = a.Name
			mv.Email = a.Email
		}
		memberViews = append(memberViews, mv)
	}

	// Build linked project IDs set for filtering.
	linkedSet := make(map[int64]bool)
	for _, tp := range teamProjects {
		linkedSet[tp.ProjectID] = true
	}

	var linkedViews []LinkedProjectView
	for _, tp := range teamProjects {
		lpv := LinkedProjectView{ProjectID: tp.ProjectID}
		for _, p := range allProjects {
			if p.ID == tp.ProjectID {
				lpv.Name = p.Name
				break
			}
		}
		if lpv.Name == "" {
			lpv.Name = fmt.Sprintf("Project #%d", tp.ProjectID)
		}
		linkedViews = append(linkedViews, lpv)
	}

	// Available accounts = those not already members.
	memberSet := make(map[int64]bool)
	for _, m := range members {
		memberSet[m.AccountID] = true
	}
	var availableAccounts []db.Account
	for _, a := range accounts {
		if !memberSet[a.ID] {
			availableAccounts = append(availableAccounts, a)
		}
	}

	// Available projects = those not already linked.
	var availableProjects []db.Project
	for _, p := range allProjects {
		if !linkedSet[p.ID] {
			availableProjects = append(availableProjects, p)
		}
	}

	detail := TeamDetailView{
		ID:                team.ID,
		Name:              team.Name,
		Slug:              team.Slug,
		Members:           memberViews,
		Projects:          linkedViews,
		AvailableAccounts: availableAccounts,
		AvailableProjects: availableProjects,
	}

	_ = teamDetailPage(account, detail).Render(r.Context(), w)
}

func (h *Handler) updateTeamForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	if err := h.DB.UpdateTeam(r.Context(), teamID, name, slug); err != nil {
		h.Log.Error("update team", "err", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/teams/%d", teamID), http.StatusSeeOther)
}

func (h *Handler) deleteTeamForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.DeleteTeam(r.Context(), teamID); err != nil {
		h.Log.Error("delete team", "err", err)
	}
	http.Redirect(w, r, "/dashboard/teams", http.StatusSeeOther)
}

func (h *Handler) addTeamMemberForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	accountID, _ := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	role := r.FormValue("role")
	if accountID > 0 {
		if err := h.DB.AddTeamMember(r.Context(), teamID, accountID, role); err != nil {
			h.Log.Error("add team member", "err", err)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/teams/%d", teamID), http.StatusSeeOther)
}

func (h *Handler) removeTeamMemberForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	accountID := pathInt64(r, "account_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.RemoveTeamMember(r.Context(), teamID, accountID); err != nil {
		h.Log.Error("remove team member", "err", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/teams/%d", teamID), http.StatusSeeOther)
}

func (h *Handler) linkTeamProjectForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	projectID, _ := strconv.ParseInt(r.FormValue("project_id"), 10, 64)
	if projectID > 0 {
		if err := h.DB.LinkTeamProject(r.Context(), teamID, projectID); err != nil {
			h.Log.Error("link team project", "err", err)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/teams/%d", teamID), http.StatusSeeOther)
}

func (h *Handler) unlinkTeamProjectForm(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	projectID := pathInt64(r, "project_id")
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.UnlinkTeamProject(r.Context(), teamID, projectID); err != nil {
		h.Log.Error("unlink team project", "err", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/teams/%d", teamID), http.StatusSeeOther)
}

// Account role management handlers.

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	flash := r.URL.Query().Get("error")

	accounts, err := h.DB.ListAccounts(r.Context())
	if err != nil {
		h.Log.Error("list accounts", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var views []AccountRoleView
	for _, a := range accounts {
		views = append(views, AccountRoleView{
			ID:    a.ID,
			Name:  a.Name,
			Email: a.Email,
			Role:  a.Role,
		})
	}

	_ = accountsPage(account, views, flash).Render(r.Context(), w)
}

func (h *Handler) updateAccountRole(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	accountID, _ := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	newRole := strings.TrimSpace(r.FormValue("role"))
	if accountID == 0 || newRole == "" {
		http.Redirect(w, r, "/dashboard/accounts", http.StatusSeeOther)
		return
	}

	// Prevent demoting the last owner.
	callerID, _ := strconv.ParseInt(r.Header.Get("X-Account-ID"), 10, 64)
	if accountID == callerID && newRole != "owner" {
		ownerCount, err := h.DB.OwnerCount(r.Context())
		if err != nil {
			h.Log.Error("owner count", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if ownerCount <= 1 {
			http.Redirect(w, r, "/dashboard/accounts?error=Cannot+demote+the+last+owner", http.StatusSeeOther)
			return
		}
	}

	if err := h.DB.UpdateAccountRole(r.Context(), accountID, newRole); err != nil {
		h.Log.Error("update account role", "err", err)
		http.Redirect(w, r, "/dashboard/accounts?error=Failed+to+update+role", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/accounts", http.StatusSeeOther)
}

// Helpers

func sessionToken(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return c.Value
}

func pathInt64(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return v
}

func queryInt(r *http.Request, name string) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return 1
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 1
	}
	return v
}

// mergeIssueForm handles the POST form to merge one issue into another.
func (h *Handler) mergeIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	sourceID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	targetID := strings.TrimSpace(r.FormValue("target_id"))
	if targetID == "" {
		http.Error(w, "target issue is required", http.StatusBadRequest)
		return
	}
	if sourceID == targetID {
		http.Error(w, "cannot merge an issue into itself", http.StatusBadRequest)
		return
	}

	if err := h.DB.MergeIssue(r.Context(), projectID, sourceID, targetID); err != nil {
		h.Log.Error("merge issue", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, sourceID), http.StatusSeeOther)
}

// unmergeIssueForm handles the POST form to reverse a merge.
func (h *Handler) unmergeIssueForm(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	sourceID := r.PathValue("issue_id")

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseForm()

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	if err := h.DB.UnmergeIssue(r.Context(), projectID, sourceID); err != nil {
		h.Log.Error("unmerge issue", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, sourceID), http.StatusSeeOther)
}

// searchIssues returns a JSON list of issues matching a query, for the merge target search.
func (h *Handler) searchIssues(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	query := r.URL.Query().Get("q")
	excludeID := r.URL.Query().Get("exclude")

	issues, _, err := h.DB.ListIssueGroups(r.Context(), db.IssueListParams{
		ProjectID: projectID,
		Query:     query,
		Limit:     10,
	})
	if err != nil {
		h.Log.Error("search issues", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	type issueResult struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Level      string `json:"level"`
		EventCount int    `json:"event_count"`
		Status     string `json:"status"`
	}
	var results []issueResult
	for _, ig := range issues {
		if ig.ID == excludeID {
			continue
		}
		results = append(results, issueResult{
			ID:         ig.ID,
			Title:      ig.Title,
			Level:      ig.Level,
			EventCount: ig.EventCount,
			Status:     ig.Status,
		})
	}
	if results == nil {
		results = []issueResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
