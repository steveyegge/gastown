package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// RTSEntity represents a single entity in the RTS game world.
type RTSEntity struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // polecat, witness, refinery, mayor, dog
	Name        string `json:"name"`
	Rig         string `json:"rig,omitempty"`
	Status      string `json:"status"`      // working, idle, stuck, stale, spawning, escalated
	Activity    string `json:"activity"`    // Human-readable current activity
	ActivityAge string `json:"activityAge"` // e.g., "2m"
	HookBead    string `json:"hookBead,omitempty"`
	HookTitle   string `json:"hookTitle,omitempty"`
	SessionAlive bool  `json:"sessionAlive"`
}

// RTSRig represents a rig zone on the map.
type RTSRig struct {
	Name         string `json:"name"`
	PolecatCount int    `json:"polecatCount"`
	CrewCount    int    `json:"crewCount"`
	HasWitness   bool   `json:"hasWitness"`
	HasRefinery  bool   `json:"hasRefinery"`
}

// RTSMergeItem represents an item in the refinery merge queue.
type RTSMergeItem struct {
	Repo     string `json:"repo"`
	Title    string `json:"title"`
	CIStatus string `json:"ciStatus"`
	Number   int    `json:"number"`
}

// RTSEscalation represents an active escalation.
type RTSEscalation struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	From     string `json:"from"`
}

// RTSState is the full game state sent to the Phaser client.
type RTSState struct {
	Timestamp   string          `json:"timestamp"`
	Entities    []RTSEntity     `json:"entities"`
	Rigs        []RTSRig        `json:"rigs"`
	MergeQueue  []RTSMergeItem  `json:"mergeQueue"`
	Escalations []RTSEscalation `json:"escalations"`
	Health      *RTSHealth      `json:"health,omitempty"`
}

// RTSHealth represents system health for the game HUD.
type RTSHealth struct {
	HealthyAgents   int  `json:"healthyAgents"`
	UnhealthyAgents int  `json:"unhealthyAgents"`
	IsPaused        bool `json:"isPaused"`
}

// RTSHandler serves the RTS game page and provides the SSE state endpoint.
type RTSHandler struct {
	fetcher      ConvoyFetcher
	fetchTimeout time.Duration
}

// NewRTSHandler creates a new RTS game handler.
func NewRTSHandler(fetcher ConvoyFetcher, fetchTimeout time.Duration) *RTSHandler {
	return &RTSHandler{
		fetcher:      fetcher,
		fetchTimeout: fetchTimeout,
	}
}

// ServeHTTP serves the RTS game page at /rts.
func (h *RTSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve the static index.html for the game
	http.ServeFile(w, r, "")
}

// HandleSSE streams game state via Server-Sent Events at /api/rts-state.
func (h *RTSHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx := r.Context()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-ticker.C:
			state := h.fetchGameState(ctx)
			data, err := json.Marshal(state)
			if err != nil {
				log.Printf("rts: failed to marshal state: %v", err)
				continue
			}
			fmt.Fprintf(w, "event: state\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// fetchGameState gathers all data from the ConvoyFetcher and transforms it into RTSState.
func (h *RTSHandler) fetchGameState(ctx context.Context) *RTSState {
	ctx, cancel := context.WithTimeout(ctx, h.fetchTimeout)
	defer cancel()

	state := &RTSState{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch workers (polecats + refineries)
	wg.Add(1)
	go func() {
		defer wg.Done()
		workers, err := h.fetcher.FetchWorkers()
		if err != nil {
			log.Printf("rts: FetchWorkers: %v", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, w := range workers {
			state.Entities = append(state.Entities, RTSEntity{
				ID:          w.SessionID,
				Type:        w.AgentType,
				Name:        w.Name,
				Rig:         w.Rig,
				Status:      w.WorkStatus,
				Activity:    w.StatusHint,
				ActivityAge: w.LastActivity.FormattedAge,
				HookBead:    w.IssueID,
				HookTitle:   w.IssueTitle,
				SessionAlive: true,
			})
		}
	}()

	// Fetch sessions (to find witnesses and dead sessions)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessions, err := h.fetcher.FetchSessions()
		if err != nil {
			log.Printf("rts: FetchSessions: %v", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, s := range sessions {
			if s.Role == "witness" || s.Role == "deacon" {
				state.Entities = append(state.Entities, RTSEntity{
					ID:           s.Name,
					Type:         s.Role,
					Name:         s.Role,
					Rig:          s.Rig,
					Status:       entityStatusFromSession(s),
					ActivityAge:  s.Activity,
					SessionAlive: s.IsAlive,
				})
			}
		}
	}()

	// Fetch mayor
	wg.Add(1)
	go func() {
		defer wg.Done()
		mayor, err := h.fetcher.FetchMayor()
		if err != nil || mayor == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		status := "idle"
		if mayor.IsActive {
			status = "working"
		}
		state.Entities = append(state.Entities, RTSEntity{
			ID:           "mayor",
			Type:         "mayor",
			Name:         "Mayor",
			Status:       status,
			ActivityAge:  mayor.LastActivity,
			SessionAlive: mayor.IsAttached,
		})
	}()

	// Fetch rigs
	wg.Add(1)
	go func() {
		defer wg.Done()
		rigs, err := h.fetcher.FetchRigs()
		if err != nil {
			log.Printf("rts: FetchRigs: %v", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, r := range rigs {
			state.Rigs = append(state.Rigs, RTSRig{
				Name:         r.Name,
				PolecatCount: r.PolecatCount,
				CrewCount:    r.CrewCount,
				HasWitness:   r.HasWitness,
				HasRefinery:  r.HasRefinery,
			})
		}
	}()

	// Fetch dogs
	wg.Add(1)
	go func() {
		defer wg.Done()
		dogs, err := h.fetcher.FetchDogs()
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, d := range dogs {
			state.Entities = append(state.Entities, RTSEntity{
				ID:          "dog-" + d.Name,
				Type:        "dog",
				Name:        d.Name,
				Status:      d.State,
				Activity:    d.Work,
				ActivityAge: d.LastActive,
			})
		}
	}()

	// Fetch merge queue
	wg.Add(1)
	go func() {
		defer wg.Done()
		mq, err := h.fetcher.FetchMergeQueue()
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, m := range mq {
			state.MergeQueue = append(state.MergeQueue, RTSMergeItem{
				Repo:     m.Repo,
				Title:    m.Title,
				CIStatus: m.CIStatus,
				Number:   m.Number,
			})
		}
	}()

	// Fetch escalations
	wg.Add(1)
	go func() {
		defer wg.Done()
		escs, err := h.fetcher.FetchEscalations()
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, e := range escs {
			if !e.Acked {
				state.Escalations = append(state.Escalations, RTSEscalation{
					ID:       e.ID,
					Title:    e.Title,
					Severity: e.Severity,
					From:     e.EscalatedBy,
				})
			}
		}
	}()

	// Fetch health
	wg.Add(1)
	go func() {
		defer wg.Done()
		health, err := h.fetcher.FetchHealth()
		if err != nil || health == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		state.Health = &RTSHealth{
			HealthyAgents:   health.HealthyAgents,
			UnhealthyAgents: health.UnhealthyAgents,
			IsPaused:        health.IsPaused,
		}
	}()

	// Fetch hooks (to enrich entity data with hook info)
	wg.Add(1)
	go func() {
		defer wg.Done()
		hooks, err := h.fetcher.FetchHooks()
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		// Store hooks for post-processing enrichment
		for _, hook := range hooks {
			// Find matching entity and enrich with hook data
			for i := range state.Entities {
				if state.Entities[i].Name == hook.Agent || state.Entities[i].ID == hook.Assignee {
					state.Entities[i].HookBead = hook.ID
					state.Entities[i].HookTitle = hook.Title
				}
			}
		}
	}()

	wg.Wait()
	return state
}

// entityStatusFromSession derives an entity status from a session.
func entityStatusFromSession(s SessionRow) string {
	if !s.IsAlive {
		return "dead"
	}
	return "working"
}
