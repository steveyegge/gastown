package web

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

//go:embed static
var staticFiles embed.FS

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
	fetcher      ConvoyFetcher
	template     *template.Template
	fetchTimeout time.Duration
}

// NewConvoyHandler creates a new convoy handler with the given fetcher and fetch timeout.
func NewConvoyHandler(fetcher ConvoyFetcher, fetchTimeout time.Duration) (*ConvoyHandler, error) {
	tmpl, err := LoadTemplates()
	if err != nil {
		return nil, err
	}

	return &ConvoyHandler{
		fetcher:      fetcher,
		template:     tmpl,
		fetchTimeout: fetchTimeout,
	}, nil
}

// ServeHTTP handles GET / requests and renders the convoy dashboard.
func (h *ConvoyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check for expand parameter (fullscreen a specific panel)
	expandPanel := r.URL.Query().Get("expand")

	// Create a timeout context for all fetches
	ctx, cancel := context.WithTimeout(r.Context(), h.fetchTimeout)
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
		log.Printf("dashboard: fetch timeout after %v", h.fetchTimeout)
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
		Issues:      enrichIssuesWithAssignees(issues, hooks),
		Activity:    activity,
		Summary:     summary,
		Expand:      expandPanel,
	}

	var buf bytes.Buffer
	if err := h.template.ExecuteTemplate(&buf, "convoy.html", data); err != nil {
		// Security: intentionally returns a generic error message to the client.
		// Internal error details (err) are not exposed in the HTTP response to
		// prevent information leakage. The error is logged server-side only.
		log.Printf("dashboard: template execution failed: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("dashboard: response write failed: %v", err)
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

// enrichIssuesWithAssignees adds Assignee info to issues by cross-referencing hooks.
func enrichIssuesWithAssignees(issues []IssueRow, hooks []HookRow) []IssueRow {
	// Build a map of issue ID -> assignee from hooks
	hookMap := make(map[string]string)
	for _, hook := range hooks {
		hookMap[hook.ID] = hook.Agent
	}

	// Enrich issues with assignee info
	for i := range issues {
		if assignee, ok := hookMap[issues[i].ID]; ok {
			issues[i].Assignee = assignee
		}
	}
	return issues
}

// NewDashboardMux creates an HTTP handler that serves both the dashboard and API.
// webCfg may be nil, in which case defaults are used.
func NewDashboardMux(fetcher ConvoyFetcher, webCfg *config.WebTimeoutsConfig) (http.Handler, error) {
	if webCfg == nil {
		webCfg = config.DefaultWebTimeoutsConfig()
	}

	fetchTimeout := config.ParseDurationOrDefault(webCfg.FetchTimeout, 8*time.Second)
	convoyHandler, err := NewConvoyHandler(fetcher, fetchTimeout)
	if err != nil {
		return nil, err
	}

	defaultRunTimeout := config.ParseDurationOrDefault(webCfg.DefaultRunTimeout, 30*time.Second)
	maxRunTimeout := config.ParseDurationOrDefault(webCfg.MaxRunTimeout, 60*time.Second)
	apiHandler := NewAPIHandler(defaultRunTimeout, maxRunTimeout)

	// Create static file server from embedded files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}
	staticHandler := http.FileServer(http.FS(staticFS))

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))
	mux.Handle("/", convoyHandler)

	return mux, nil
}
