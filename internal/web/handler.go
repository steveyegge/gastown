package web

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strings"
)

// renderDetailError writes an HTML error message that htmx will display.
// Uses 200 status because htmx ignores non-2xx responses by default.
func renderDetailError(w http.ResponseWriter, context string, err error) {
	log.Printf("Error %s : %v", context, err)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="detail-error" style="padding: 16px; background: rgba(255,100,100,0.1); border-left: 3px solid var(--red, #f66); margin: 8px 0;">
		<strong style="color: var(--red, #f66);">Error %s:</strong>
		<pre style="margin: 8px 0 0 0; white-space: pre-wrap; font-size: 0.85em;">%s</pre>
	</div>`, html.EscapeString(context), html.EscapeString(err.Error()))
}

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
	fetcher     DetailFetcher
	template    *template.Template
	beadsUIPort int // Port for beads-ui iframe (0 if not available)
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

// SetBeadsUIPort sets the port for the beads-ui iframe.
func (h *ConvoyHandler) SetBeadsUIPort(port int) {
	h.beadsUIPort = port
}

// ServeHTTP handles GET / requests and renders the convoy dashboard.
func (h *ConvoyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var errors []string

	convoys, err := h.fetcher.FetchConvoys()
	if err != nil {
		log.Printf("Error fetching convoys: %v", err)
		errors = append(errors, fmt.Sprintf("Convoys: %v", err))
		convoys = nil // Show empty instead of crashing
	}

	// Note: MergeQueue (GitHub PRs) is deprecated in favor of InternalMRs (beads)
	// Keeping the field for backward compatibility but not fetching from GitHub anymore

	polecats, err := h.fetcher.FetchPolecats()
	if err != nil {
		log.Printf("Error fetching polecats: %v", err)
		errors = append(errors, fmt.Sprintf("Polecats: %v", err))
		polecats = nil
	}

	mergeHistory, err := h.fetcher.FetchMergeHistory(10)
	if err != nil {
		log.Printf("Error fetching merge history: %v", err)
		errors = append(errors, fmt.Sprintf("Merge History: %v", err))
		mergeHistory = nil
	}

	activity, err := h.fetcher.FetchActivity(20)
	if err != nil {
		log.Printf("Error fetching activity: %v", err)
		errors = append(errors, fmt.Sprintf("Activity: %v", err))
		activity = nil
	}

	hqAgents, err := h.fetcher.FetchHQAgents()
	if err != nil {
		log.Printf("Error fetching HQ agents: %v", err)
		errors = append(errors, fmt.Sprintf("HQ Agents: %v", err))
		hqAgents = nil
	}

	internalMRs, err := h.fetcher.FetchInternalMRs()
	if err != nil {
		log.Printf("Error fetching internal MRs: %v", err)
		errors = append(errors, fmt.Sprintf("Internal MRs: %v", err))
		internalMRs = nil
	}

	data := ConvoyData{
		Convoys:      convoys,
		InternalMRs:  internalMRs,
		MergeHistory: mergeHistory,
		Polecats:     polecats,
		Activity:     activity,
		HQAgents:     hqAgents,
		BeadsUIPort:  h.beadsUIPort,
		Errors:       errors,
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
		renderDetailError(w, "fetching polecat "+sessionID, err)
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
		renderDetailError(w, "fetching convoy "+convoyID, err)
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
		renderDetailError(w, "fetching HQ agent "+sessionID, err)
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
