package web

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/workspace"
)

//go:embed static
var staticFiles embed.FS

// fetchTimeout is the maximum time allowed for all data fetches to complete.
const fetchTimeout = 8 * time.Second

// ConvoyFetcher defines the interface for fetching convoy data.
type ConvoyFetcher interface {
	FetchConvoys() ([]ConvoyRow, error)
	FetchMergeQueue() ([]MergeQueueRow, error)
	FetchWorkers() ([]WorkerRow, error)
	FetchMail() ([]MailRow, error)
	FetchRigs() ([]RigRow, error)
	FetchDogs() ([]DogRow, error)
	FetchEscalations() ([]EscalationRow, error)
	FetchHealth() (*HealthRow, error)
	FetchQueues() ([]QueueRow, error)
	FetchSessions() ([]SessionRow, error)
	FetchHooks() ([]HookRow, error)
	FetchMayor() (*MayorStatus, error)
	FetchIssues() ([]IssueRow, error)
	FetchActivity() ([]ActivityRow, error)
}

// ConvoyHandler handles HTTP requests for the convoy dashboard.
type ConvoyHandler struct {
	fetcher  ConvoyFetcher
	template *template.Template
}

// NewConvoyHandler creates a new convoy handler with the given fetcher.
func NewConvoyHandler(fetcher ConvoyFetcher) (*ConvoyHandler, error) {
	tmpl, err := LoadTemplates()
	if err != nil {
		return nil, err
	}

	return &ConvoyHandler{
		fetcher:  fetcher,
		template: tmpl,
	}, nil
}

// ServeHTTP handles GET / requests and renders the convoy dashboard.
func (h *ConvoyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check for expand parameter (fullscreen a specific panel)
	expandPanel := r.URL.Query().Get("expand")

	// Create a timeout context for all fetches
	ctx, cancel := context.WithTimeout(r.Context(), fetchTimeout)
	defer cancel()

	var (
		convoys     []ConvoyRow
		mergeQueue  []MergeQueueRow
		workers     []WorkerRow
		mail        []MailRow
		rigs        []RigRow
		dogs        []DogRow
		escalations []EscalationRow
		health      *HealthRow
		queues      []QueueRow
		sessions    []SessionRow
		hooks       []HookRow
		mayor       *MayorStatus
		issues      []IssueRow
		activity    []ActivityRow
		wg          sync.WaitGroup
	)

	// Run all fetches in parallel with error logging
	wg.Add(14)

	go func() {
		defer wg.Done()
		var err error
		convoys, err = h.fetcher.FetchConvoys()
		if err != nil {
			log.Printf("dashboard: FetchConvoys failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		mergeQueue, err = h.fetcher.FetchMergeQueue()
		if err != nil {
			log.Printf("dashboard: FetchMergeQueue failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		workers, err = h.fetcher.FetchWorkers()
		if err != nil {
			log.Printf("dashboard: FetchWorkers failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		mail, err = h.fetcher.FetchMail()
		if err != nil {
			log.Printf("dashboard: FetchMail failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		rigs, err = h.fetcher.FetchRigs()
		if err != nil {
			log.Printf("dashboard: FetchRigs failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		dogs, err = h.fetcher.FetchDogs()
		if err != nil {
			log.Printf("dashboard: FetchDogs failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		escalations, err = h.fetcher.FetchEscalations()
		if err != nil {
			log.Printf("dashboard: FetchEscalations failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		health, err = h.fetcher.FetchHealth()
		if err != nil {
			log.Printf("dashboard: FetchHealth failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		queues, err = h.fetcher.FetchQueues()
		if err != nil {
			log.Printf("dashboard: FetchQueues failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		sessions, err = h.fetcher.FetchSessions()
		if err != nil {
			log.Printf("dashboard: FetchSessions failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		hooks, err = h.fetcher.FetchHooks()
		if err != nil {
			log.Printf("dashboard: FetchHooks failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		mayor, err = h.fetcher.FetchMayor()
		if err != nil {
			log.Printf("dashboard: FetchMayor failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		issues, err = h.fetcher.FetchIssues()
		if err != nil {
			log.Printf("dashboard: FetchIssues failed: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		activity, err = h.fetcher.FetchActivity()
		if err != nil {
			log.Printf("dashboard: FetchActivity failed: %v", err)
		}
	}()

	// Wait for fetches or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All fetches completed
	case <-ctx.Done():
		log.Printf("dashboard: fetch timeout after %v", fetchTimeout)
	}

	// Compute summary from already-fetched data
	summary := computeSummary(workers, hooks, issues, convoys, escalations, activity)

	data := ConvoyData{
		Convoys:     convoys,
		MergeQueue:  mergeQueue,
		Workers:     workers,
		Mail:        mail,
		Rigs:        rigs,
		Dogs:        dogs,
		Escalations: escalations,
		Health:      health,
		Queues:      queues,
		Sessions:    sessions,
		Hooks:       hooks,
		Mayor:       mayor,
		Issues:      issues,
		Activity:    activity,
		Summary:     summary,
		Expand:      expandPanel,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := h.template.ExecuteTemplate(w, "convoy.html", data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// computeSummary calculates dashboard stats and alerts from fetched data.
func computeSummary(workers []WorkerRow, hooks []HookRow, issues []IssueRow,
	convoys []ConvoyRow, escalations []EscalationRow, activity []ActivityRow) *DashboardSummary {

	summary := &DashboardSummary{
		PolecatCount:    len(workers),
		HookCount:       len(hooks),
		IssueCount:      len(issues),
		ConvoyCount:     len(convoys),
		EscalationCount: len(escalations),
	}

	// Count stuck workers (status = "stuck")
	for _, w := range workers {
		if w.WorkStatus == "stuck" {
			summary.StuckPolecats++
		}
	}

	// Count stale hooks (IsStale = true)
	for _, h := range hooks {
		if h.IsStale {
			summary.StaleHooks++
		}
	}

	// Count unacked escalations
	for _, e := range escalations {
		if !e.Acked {
			summary.UnackedEscalations++
		}
	}

	// Count high priority issues (P1 or P2)
	for _, i := range issues {
		if i.Priority == 1 || i.Priority == 2 {
			summary.HighPriorityIssues++
		}
	}

	// Count recent session deaths from activity
	for _, a := range activity {
		if a.Type == "session_death" || a.Type == "mass_death" {
			summary.DeadSessions++
		}
	}

	// Set HasAlerts flag
	summary.HasAlerts = summary.StuckPolecats > 0 ||
		summary.StaleHooks > 0 ||
		summary.UnackedEscalations > 0 ||
		summary.DeadSessions > 0 ||
		summary.HighPriorityIssues > 0

	return summary
}

// NewDashboardMux creates an HTTP handler that serves both the dashboard and API.
func NewDashboardMux(fetcher ConvoyFetcher) (http.Handler, error) {
	convoyHandler, err := NewConvoyHandler(fetcher)
	if err != nil {
		return nil, err
	}

	apiHandler := NewAPIHandler()

	// Create static file server from embedded files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	staticHandler := http.FileServer(http.FS(staticFS))

	// Create artifacts handler for serving Playwright reports and snapshots
	artifactsHandler, err := NewArtifactsHandler()
	if err != nil {
		// Non-fatal: dashboard can work without artifacts
		log.Printf("dashboard: artifacts handler unavailable: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	if artifactsHandler != nil {
		mux.Handle("/artifacts/", artifactsHandler)
	}
	mux.Handle("/", convoyHandler)

	return mux, nil
}

// ArtifactsHandler serves Playwright test artifacts (reports, screenshots, etc.)
type ArtifactsHandler struct {
	artifactsDir string
}

// NewArtifactsHandler creates a handler for serving test artifacts.
// Artifacts are stored at <town>/.beads/artifacts/
func NewArtifactsHandler() (*ArtifactsHandler, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, err
	}

	artifactsDir := filepath.Join(townRoot, ".beads", "artifacts")
	return &ArtifactsHandler{
		artifactsDir: artifactsDir,
	}, nil
}

// ServeHTTP handles requests for artifact files.
// URL format: /artifacts/{convoy-id}/report -> serves playwright-report/index.html
//
//	/artifacts/{convoy-id}/snapshots/{path} -> serves test snapshots
//	/artifacts/{convoy-id}/results/{path} -> serves test results (screenshots, traces)
func (h *ArtifactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse URL path: /artifacts/{convoy-id}/{type}[/{path}]
	path := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	convoyID := parts[0]
	resourceType := parts[1]

	// Validate convoy ID (prevent directory traversal)
	if strings.Contains(convoyID, "..") || strings.Contains(convoyID, "/") {
		http.Error(w, "Invalid convoy ID", http.StatusBadRequest)
		return
	}

	var filePath string
	switch resourceType {
	case "report":
		// Serve the HTML report
		filePath = filepath.Join(h.artifactsDir, "convoys", convoyID, "playwright-report", "index.html")
	case "snapshots":
		// Serve snapshot images
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		subPath := parts[2]
		// Prevent directory traversal
		if strings.Contains(subPath, "..") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		filePath = filepath.Join(h.artifactsDir, "convoys", convoyID, "test-results", subPath)
	case "results":
		// Serve any file from test-results (screenshots, traces, etc.)
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		subPath := parts[2]
		if strings.Contains(subPath, "..") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		filePath = filepath.Join(h.artifactsDir, "convoys", convoyID, "test-results", subPath)
	case "report-assets":
		// Serve assets needed by the HTML report (css, js, etc.)
		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		subPath := parts[2]
		if strings.Contains(subPath, "..") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		filePath = filepath.Join(h.artifactsDir, "convoys", convoyID, "playwright-report", subPath)
	default:
		http.NotFound(w, r)
		return
	}

	// Serve the file
	http.ServeFile(w, r, filePath)
}
