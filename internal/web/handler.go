package web

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"strings"
)

// ConvoyFetcher defines the interface for fetching convoy data.
type ConvoyFetcher interface {
	FetchConvoys() ([]ConvoyRow, error)
	FetchMergeQueue() ([]MergeQueueRow, error)
	FetchPolecats() ([]PolecatRow, error)
}

// DetailFetcher extends ConvoyFetcher with methods for expanded details.
type DetailFetcher interface {
	ConvoyFetcher
	FetchPolecatDetail(sessionID string) (*PolecatDetail, error)
	FetchConvoyDetail(convoyID string) (*ConvoyDetail, error)
	FetchMergeHistory(limit int) ([]MergeHistoryRow, error)
	FetchActivity(limit int) ([]ActivityEvent, error)
	FetchHQAgents() ([]HQAgentRow, error)
	FetchHQAgentDetail(sessionID string) (*HQAgentDetail, error)
	FetchInternalMRs() ([]InternalMRRow, error)
}

// ConvoyHandler handles HTTP requests for the convoy dashboard.
type ConvoyHandler struct {
	fetcher  DetailFetcher
	template *template.Template
}

// NewConvoyHandler creates a new convoy handler with the given fetcher.
func NewConvoyHandler(fetcher DetailFetcher) (*ConvoyHandler, error) {
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
	convoys, err := h.fetcher.FetchConvoys()
	if err != nil {
		http.Error(w, "Failed to fetch convoys", http.StatusInternalServerError)
		return
	}

	// Note: MergeQueue (GitHub PRs) is deprecated in favor of InternalMRs (beads)
	// Keeping the field for backward compatibility but not fetching from GitHub anymore

	polecats, err := h.fetcher.FetchPolecats()
	if err != nil {
		// Non-fatal: show convoys even if polecats fail
		polecats = nil
	}

	mergeHistory, err := h.fetcher.FetchMergeHistory(10)
	if err != nil {
		// Non-fatal: show dashboard even if merge history fails
		mergeHistory = nil
	}

	activity, err := h.fetcher.FetchActivity(20)
	if err != nil {
		// Non-fatal: show dashboard even if activity fails
		activity = nil
	}

	hqAgents, err := h.fetcher.FetchHQAgents()
	if err != nil {
		// Non-fatal: show dashboard even if HQ agents fail
		hqAgents = nil
	}

	internalMRs, err := h.fetcher.FetchInternalMRs()
	if err != nil {
		// Non-fatal: show dashboard even if internal MRs fail
		internalMRs = nil
	}

	data := ConvoyData{
		Convoys:      convoys,
		InternalMRs:  internalMRs,
		MergeHistory: mergeHistory,
		Polecats:     polecats,
		Activity:     activity,
		HQAgents:     hqAgents,
	}

	// Execute to buffer first to avoid partial writes on error
	var buf bytes.Buffer
	if err := h.template.ExecuteTemplate(&buf, "convoy.html", data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// ServePolecatDetail handles GET /polecat/{session}/details requests.
func (h *ConvoyHandler) ServePolecatDetail(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /polecat/{session}/details
	path := strings.TrimPrefix(r.URL.Path, "/polecat/")
	sessionID := strings.TrimSuffix(path, "/details")

	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	detail, err := h.fetcher.FetchPolecatDetail(sessionID)
	if err != nil {
		http.Error(w, "Failed to fetch polecat details", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := h.template.ExecuteTemplate(&buf, "polecat_detail.html", detail); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// ServeConvoyDetail handles GET /convoy/{id}/details requests.
func (h *ConvoyHandler) ServeConvoyDetail(w http.ResponseWriter, r *http.Request) {
	// Extract convoy ID from path: /convoy/{id}/details
	path := strings.TrimPrefix(r.URL.Path, "/convoy/")
	convoyID := strings.TrimSuffix(path, "/details")

	if convoyID == "" {
		http.Error(w, "Convoy ID required", http.StatusBadRequest)
		return
	}

	detail, err := h.fetcher.FetchConvoyDetail(convoyID)
	if err != nil {
		http.Error(w, "Failed to fetch convoy details", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := h.template.ExecuteTemplate(&buf, "convoy_detail.html", detail); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// ServeHQDetail handles GET /hq/{session}/details requests.
func (h *ConvoyHandler) ServeHQDetail(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /hq/{session}/details
	path := strings.TrimPrefix(r.URL.Path, "/hq/")
	sessionID := strings.TrimSuffix(path, "/details")

	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	detail, err := h.fetcher.FetchHQAgentDetail(sessionID)
	if err != nil {
		http.Error(w, "Failed to fetch HQ agent details", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := h.template.ExecuteTemplate(&buf, "hq_detail.html", detail); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Failed to fetch HQ agent details", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
