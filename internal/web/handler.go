package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
)

// ConvoyFetcher defines the interface for fetching convoy data.
type ConvoyFetcher interface {
	FetchConvoys() ([]ConvoyRow, error)
	FetchMergeQueue() ([]MergeQueueRow, error)
	FetchPolecats() ([]PolecatRow, error)
	FetchPolecatDetail(rig, name string) (*PolecatDetailData, error)
}

// ConvoyHandler handles HTTP requests for the convoy dashboard.
// It also serves as a multiplexer for /feed SSE endpoint.
type ConvoyHandler struct {
	fetcher  ConvoyFetcher
	template *template.Template
	townRoot string
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

// SetTownRoot sets the town root for the activity watcher.
func (h *ConvoyHandler) SetTownRoot(townRoot string) {
	h.townRoot = townRoot
}

// ServeHTTP handles HTTP requests and routes to appropriate handlers.
func (h *ConvoyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/feed":
		h.serveFeed(w, r)
	default:
		h.serveConvoyDashboard(w, r)
	}
}

// serveConvoyDashboard handles GET / requests and renders the convoy dashboard.
func (h *ConvoyHandler) serveConvoyDashboard(w http.ResponseWriter, r *http.Request) {
	convoys, err := h.fetcher.FetchConvoys()
	if err != nil {
		http.Error(w, "Failed to fetch convoys", http.StatusInternalServerError)
		return
	}

	mergeQueue, err := h.fetcher.FetchMergeQueue()
	if err != nil {
		// Non-fatal: show convoys even if merge queue fails
		mergeQueue = nil
	}

	polecats, err := h.fetcher.FetchPolecats()
	if err != nil {
		// Non-fatal: show convoys even if polecats fail
		polecats = nil
	}

	// Calculate total cost from all polecat sessions
	var totalCost float64
	for _, p := range polecats {
		totalCost += p.SessionCost
	}

	// Group polecats by rig for progressive disclosure view
	rigGroups := groupPolecatsByRig(polecats)

	data := ConvoyData{
		Convoys:    convoys,
		MergeQueue: mergeQueue,
		Polecats:   polecats,
		TotalCost:  totalCost,
		RigGroups:  rigGroups,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := h.template.ExecuteTemplate(w, "convoy.html", data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// ServePolecatDetail handles GET /polecat/{rig}/{name} requests.
func (h *ConvoyHandler) ServePolecatDetail(w http.ResponseWriter, r *http.Request, rig, name string) {
	detail, err := h.fetcher.FetchPolecatDetail(rig, name)
	if err != nil {
		http.Error(w, "Polecat not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := h.template.ExecuteTemplate(w, "polecat_detail.html", detail); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
		return
	}
}

// serveFeed handles GET /feed requests with Server-Sent Events (SSE).
func (h *ConvoyHandler) serveFeed(w http.ResponseWriter, r *http.Request) {
	// Check if the client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create activity watcher
	watcher, err := NewActivityWatcher(h.townRoot)
	if err != nil {
		// Send error event then close
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer func() { _ = watcher.Close() }()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Stream events until client disconnects
	ctx := r.Context()
	events := watcher.Events()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return

		case event, ok := <-events:
			if !ok {
				// Event channel closed
				return
			}

			// Marshal event to JSON
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			// Send SSE event
			// Format: event: <type>\ndata: <json>\nid: <id>\n\n
			fmt.Fprintf(w, "event: %s\ndata: %s\nid: %s\n\n", event.Type, string(data), event.ID)
			flusher.Flush()
		}
	}
}

// groupPolecatsByRig groups polecats by their rig for progressive disclosure display.
// Returns a sorted slice of RigGroups, with rigs sorted alphabetically and
// polecats within each rig sorted by name.
func groupPolecatsByRig(polecats []PolecatRow) []RigGroup {
	if len(polecats) == 0 {
		return nil
	}

	// Group polecats by rig
	rigMap := make(map[string][]PolecatRow)
	for _, p := range polecats {
		rigMap[p.Rig] = append(rigMap[p.Rig], p)
	}

	// Sort polecats within each rig by name
	for rig := range rigMap {
		rigPolecats := rigMap[rig]
		sort.Slice(rigPolecats, func(i, j int) bool {
			return rigPolecats[i].Name < rigPolecats[j].Name
		})
		rigMap[rig] = rigPolecats
	}

	// Build sorted slice of RigGroups
	rigNames := make([]string, 0, len(rigMap))
	for rig := range rigMap {
		rigNames = append(rigNames, rig)
	}
	sort.Strings(rigNames)

	groups := make([]RigGroup, 0, len(rigNames))
	for _, rig := range rigNames {
		groups = append(groups, RigGroup{
			Name:     rig,
			Polecats: rigMap[rig],
		})
	}

	return groups
}
