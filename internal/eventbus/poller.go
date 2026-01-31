// Package eventbus provides an in-process pub/sub event bus for decision events.
package eventbus

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// DecisionData contains decision information for publishing events.
// This is a simple struct that avoids importing proto types.
type DecisionData struct {
	ID              string
	Question        string
	Context         string
	RequestedBy     string
	Urgency         string
	Blockers        []string
	Options         []DecisionOptionData
	ParentBeadID    string // Parent bead ID (e.g., epic) for hierarchy/routing
	ParentBeadTitle string // Parent bead title for channel derivation
}

// DecisionOptionData represents an option in a decision.
type DecisionOptionData struct {
	Label       string
	Description string
	Recommended bool
}

// DecisionPublisher is a function that publishes a new decision to the event bus.
// This abstraction allows the poller to work without importing proto types.
type DecisionPublisher func(data DecisionData)

// DecisionPoller polls the database for new decisions and publishes them to the event bus.
// This catches decisions created via CLI (bd decision create, gt decision request fallback)
// that bypass the RPC layer and event bus.
type DecisionPoller struct {
	publisher DecisionPublisher
	beadsPath string
	interval  time.Duration
	seen      map[string]bool
	seenMu    sync.Mutex
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewDecisionPoller creates a new decision poller.
// The publisher function is called for each newly discovered decision.
func NewDecisionPoller(publisher DecisionPublisher, beadsPath string, interval time.Duration) *DecisionPoller {
	return &DecisionPoller{
		publisher: publisher,
		beadsPath: beadsPath,
		interval:  interval,
		seen:      make(map[string]bool),
	}
}

// Start begins polling for new decisions. Call Stop() to shut down.
func (p *DecisionPoller) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.run(ctx)
}

// Stop shuts down the poller and waits for it to finish.
func (p *DecisionPoller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
}

// MarkSeen marks a decision ID as already seen, preventing duplicate notifications.
// Call this when a decision is created through the RPC layer (already published to bus).
func (p *DecisionPoller) MarkSeen(decisionID string) {
	p.seenMu.Lock()
	defer p.seenMu.Unlock()
	p.seen[decisionID] = true
}

func (p *DecisionPoller) run(ctx context.Context) {
	defer p.wg.Done()

	client := beads.New(p.beadsPath)

	// Do an initial poll to seed the seen set (don't notify for existing decisions)
	p.seedSeen(client)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(client)
		}
	}
}

// seedSeen populates the seen set with existing decisions to avoid notifying
// for decisions that existed before the poller started.
func (p *DecisionPoller) seedSeen(client *beads.Beads) {
	issues, err := client.ListDecisions()
	if err != nil {
		log.Printf("DecisionPoller: error seeding seen set: %v", err)
		return
	}

	p.seenMu.Lock()
	defer p.seenMu.Unlock()
	for _, issue := range issues {
		p.seen[issue.ID] = true
	}
	log.Printf("DecisionPoller: seeded with %d existing decisions", len(issues))
}

func (p *DecisionPoller) poll(client *beads.Beads) {
	issues, err := client.ListDecisions()
	if err != nil {
		// Silently ignore errors - the database might be temporarily unavailable
		return
	}

	for _, issue := range issues {
		p.seenMu.Lock()
		alreadySeen := p.seen[issue.ID]
		if !alreadySeen {
			p.seen[issue.ID] = true
		}
		p.seenMu.Unlock()

		if alreadySeen {
			continue
		}

		// New decision found - fetch full details and publish
		_, fields, err := client.GetDecisionBead(issue.ID)
		if err != nil || fields == nil {
			log.Printf("DecisionPoller: error fetching decision %s: %v", issue.ID, err)
			continue
		}

		// Convert to DecisionData
		data := DecisionData{
			ID:              issue.ID,
			Question:        fields.Question,
			Context:         fields.Context,
			RequestedBy:     fields.RequestedBy,
			Urgency:         fields.Urgency,
			Blockers:        fields.Blockers,
			ParentBeadID:    fields.ParentBeadID,
			ParentBeadTitle: fields.ParentBeadTitle,
		}
		for _, opt := range fields.Options {
			data.Options = append(data.Options, DecisionOptionData{
				Label:       opt.Label,
				Description: opt.Description,
				Recommended: opt.Recommended,
			})
		}

		log.Printf("DecisionPoller: publishing new CLI-created decision %s: %q (parent: %s)", issue.ID, fields.Question, fields.ParentBeadID)
		p.publisher(data)
	}
}
