package web

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
)

// fetchTimeout is the maximum time allowed for all data fetches to complete.
const fetchTimeout = 8 * time.Second

// ConvoyFetcher defines the interface for fetching convoy data.
type ConvoyFetcher interface {
	FetchConvoys() ([]ConvoyRow, error)
	FetchMergeQueue() ([]MergeQueueRow, error)
	FetchPolecats() ([]PolecatRow, error)
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
		polecats    []PolecatRow
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
		errMu       sync.Mutex
		errors      = make(map[string]string)
	)

	// Run all fetches in parallel with error logging
	wg.Add(14)

	// Helper to record fetch errors
	recordErr := func(panel string, err error) {
		if err != nil {
			log.Printf("dashboard: Fetch%s failed: %v", panel, err)
			errMu.Lock()
			errors[panel] = err.Error()
			errMu.Unlock()
		}
	}

	go func() {
		defer wg.Done()
		var err error
		convoys, err = h.fetcher.FetchConvoys()
		recordErr("Convoys", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		mergeQueue, err = h.fetcher.FetchMergeQueue()
		recordErr("MergeQueue", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		polecats, err = h.fetcher.FetchPolecats()
		recordErr("Polecats", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		mail, err = h.fetcher.FetchMail()
		recordErr("Mail", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		rigs, err = h.fetcher.FetchRigs()
		recordErr("Rigs", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		dogs, err = h.fetcher.FetchDogs()
		recordErr("Dogs", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		escalations, err = h.fetcher.FetchEscalations()
		recordErr("Escalations", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		health, err = h.fetcher.FetchHealth()
		recordErr("Health", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		queues, err = h.fetcher.FetchQueues()
		recordErr("Queues", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		sessions, err = h.fetcher.FetchSessions()
		recordErr("Sessions", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		hooks, err = h.fetcher.FetchHooks()
		recordErr("Hooks", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		mayor, err = h.fetcher.FetchMayor()
		recordErr("Mayor", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		issues, err = h.fetcher.FetchIssues()
		recordErr("Issues", err)
	}()
	go func() {
		defer wg.Done()
		var err error
		activity, err = h.fetcher.FetchActivity()
		recordErr("Activity", err)
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
	summary := computeSummary(polecats, hooks, issues, convoys, escalations, activity)

	data := ConvoyData{
		Convoys:     convoys,
		MergeQueue:  mergeQueue,
		Polecats:    polecats,
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
		Errors:      errors,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := h.template.ExecuteTemplate(w, "convoy.html", data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// computeSummary calculates dashboard stats and alerts from fetched data.
func computeSummary(polecats []PolecatRow, hooks []HookRow, issues []IssueRow,
	convoys []ConvoyRow, escalations []EscalationRow, activity []ActivityRow) *DashboardSummary {

	summary := &DashboardSummary{
		PolecatCount:    len(polecats),
		HookCount:       len(hooks),
		IssueCount:      len(issues),
		ConvoyCount:     len(convoys),
		EscalationCount: len(escalations),
	}

	// Count stuck polecats (status = "stuck")
	for _, p := range polecats {
		if p.WorkStatus == "stuck" {
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
