// Package dashboard serves the faultline web UI.
package dashboard

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/outdoorsea/faultline/internal/db"
)

//go:embed static
var staticFS embed.FS

// Handler serves the dashboard UI.
type Handler struct {
	DB  *db.DB
	Log *slog.Logger
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

	// Protected routes (session required).
	mux.HandleFunc("GET /dashboard/", h.requireAuth(h.index))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues", h.requireAuth(h.listIssues))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/", h.requireAuth(h.listIssues))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/{issue_id}", h.requireAuth(h.showIssue))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/issues/{issue_id}/", h.requireAuth(h.showIssue))
	mux.HandleFunc("POST /dashboard/projects/{project_id}/issues/{issue_id}/resolve", h.requireAuth(h.resolveIssue))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/events/{event_id}", h.requireAuth(h.showEvent))
	mux.HandleFunc("GET /dashboard/projects/{project_id}/events/{event_id}/", h.requireAuth(h.showEvent))
}

// requireAuth wraps a handler with session authentication.
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		if token == "" {
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		account, err := h.DB.GetSession(r.Context(), token)
		if err != nil || account == nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		// Store account in context via header (simple approach).
		r.Header.Set("X-Account-Name", account.Name)
		r.Header.Set("X-Account-ID", strconv.FormatInt(account.ID, 10))
		next(w, r)
	}
}

func (h *Handler) currentAccount(r *http.Request) *AccountView {
	name := r.Header.Get("X-Account-Name")
	if name == "" {
		return nil
	}
	return &AccountView{Name: name}
}

// Auth handlers

func (h *Handler) showLogin(w http.ResponseWriter, r *http.Request) {
	// Redirect to setup if no accounts exist.
	count, _ := h.DB.AccountCount(r.Context())
	if count == 0 {
		http.Redirect(w, r, "/dashboard/setup", http.StatusSeeOther)
		return
	}
	loginPage("").Render(r.Context(), w)
}

func (h *Handler) doLogin(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	account, err := h.DB.Authenticate(r.Context(), email, password)
	if err != nil {
		loginPage("Invalid email or password.").Render(r.Context(), w)
		return
	}

	token, err := h.DB.CreateSession(r.Context(), account.ID)
	if err != nil {
		h.Log.Error("create session", "err", err)
		loginPage("Internal error.").Render(r.Context(), w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

func (h *Handler) showSetup(w http.ResponseWriter, r *http.Request) {
	count, _ := h.DB.AccountCount(r.Context())
	if count > 0 {
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
		return
	}
	setupPage("").Render(r.Context(), w)
}

func (h *Handler) doSetup(w http.ResponseWriter, r *http.Request) {
	count, _ := h.DB.AccountCount(r.Context())
	if count > 0 {
		http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
		return
	}

	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if name == "" || email == "" || len(password) < 8 {
		setupPage("Name, email required. Password must be 8+ characters.").Render(r.Context(), w)
		return
	}

	account, err := h.DB.CreateAccount(r.Context(), email, name, password, "owner")
	if err != nil {
		h.Log.Error("create account", "err", err)
		setupPage("Could not create account.").Render(r.Context(), w)
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
		h.DB.DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", MaxAge: -1, Path: "/"})
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// Dashboard handlers

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	// Default to project 1 for now.
	http.Redirect(w, r, "/dashboard/projects/1/issues", http.StatusSeeOther)
}

func (h *Handler) listIssues(w http.ResponseWriter, r *http.Request) {
	account := h.currentAccount(r)
	projectID := pathInt64(r, "project_id")
	status := r.URL.Query().Get("status")
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}

	params := db.IssueListParams{
		ProjectID: projectID,
		Status:    status,
		Limit:     25,
		Offset:    (page - 1) * 25,
	}

	issues, total, err := h.DB.ListIssueGroups(r.Context(), params)
	if err != nil {
		h.Log.Error("list issues", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var views []IssueView
	for _, ig := range issues {
		views = append(views, IssueView{
			ID:         ig.ID,
			ProjectID:  ig.ProjectID,
			Title:      ig.Title,
			Culprit:    ig.Culprit,
			Level:      ig.Level,
			Status:     ig.Status,
			FirstSeen:  ig.FirstSeen,
			LastSeen:   ig.LastSeen,
			EventCount: ig.EventCount,
		})
	}

	seismographPage(account, views, total, projectID, status, page).Render(r.Context(), w)
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

	events, err := h.DB.ListEventsByGroup(r.Context(), projectID, issueID, 20, 0)
	if err != nil {
		h.Log.Error("list events", "err", err)
	}

	iv := IssueView{
		ID: issue.ID, ProjectID: issue.ProjectID,
		Title: issue.Title, Culprit: issue.Culprit, Level: issue.Level,
		Status: issue.Status, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen,
		EventCount: issue.EventCount,
	}

	var evs []EventView
	for _, e := range events {
		evs = append(evs, EventView{
			EventID: e.EventID, Timestamp: e.Timestamp,
			Platform: e.Platform, Level: e.Level, Message: e.Message,
		})
	}

	faultReportPage(account, iv, evs, projectID).Render(r.Context(), w)
}

func (h *Handler) resolveIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	if err := h.DB.ResolveIssueGroup(r.Context(), projectID, issueID); err != nil {
		h.Log.Error("resolve issue", "err", err)
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
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

	coreSamplePage(account, ev, projectID, event.GroupID).Render(r.Context(), w)
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
