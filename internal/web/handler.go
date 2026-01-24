package web

import (
	"html/template"
	"net/http"
	"sync"
)

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
	)

	// Run all fetches in parallel
	wg.Add(14)

	go func() { defer wg.Done(); convoys, _ = h.fetcher.FetchConvoys() }()
	go func() { defer wg.Done(); mergeQueue, _ = h.fetcher.FetchMergeQueue() }()
	go func() { defer wg.Done(); polecats, _ = h.fetcher.FetchPolecats() }()
	go func() { defer wg.Done(); mail, _ = h.fetcher.FetchMail() }()
	go func() { defer wg.Done(); rigs, _ = h.fetcher.FetchRigs() }()
	go func() { defer wg.Done(); dogs, _ = h.fetcher.FetchDogs() }()
	go func() { defer wg.Done(); escalations, _ = h.fetcher.FetchEscalations() }()
	go func() { defer wg.Done(); health, _ = h.fetcher.FetchHealth() }()
	go func() { defer wg.Done(); queues, _ = h.fetcher.FetchQueues() }()
	go func() { defer wg.Done(); sessions, _ = h.fetcher.FetchSessions() }()
	go func() { defer wg.Done(); hooks, _ = h.fetcher.FetchHooks() }()
	go func() { defer wg.Done(); mayor, _ = h.fetcher.FetchMayor() }()
	go func() { defer wg.Done(); issues, _ = h.fetcher.FetchIssues() }()
	go func() { defer wg.Done(); activity, _ = h.fetcher.FetchActivity() }()

	wg.Wait()

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
