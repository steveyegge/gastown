package poller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// RelayEnvelope matches the relay's poll response envelope format.
type RelayEnvelope struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	PublicKey string `json:"public_key"`
	Payload   []byte `json:"payload"`
}

// IngestFunc processes a raw envelope payload for a project.
// This is typically the faultline ingest handler's internal processing function.
type IngestFunc func(ctx context.Context, projectID int64, publicKey string, payload []byte) error

// RelayPoller periodically pulls envelopes from a public relay and feeds
// them through the local faultline ingest pipeline.
// CIWebhookFunc processes a raw CI webhook payload from the relay.
type CIWebhookFunc func(ctx context.Context, payload []byte) error

type RelayPoller struct {
	relayURL  string
	interval  time.Duration
	ingest    IngestFunc
	onCIWebhook CIWebhookFunc
	log       *slog.Logger
	client    *http.Client
	lastID    int64
	pollToken string
}

// NewRelayPoller creates a poller for the given relay URL.
func NewRelayPoller(relayURL string, interval time.Duration, ingest IngestFunc, log *slog.Logger) *RelayPoller {
	return &RelayPoller{
		relayURL: relayURL,
		interval: interval,
		ingest:   ingest,
		log:      log,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// SetPollToken sets the bearer token for authenticating poll/ack requests.
func (p *RelayPoller) SetPollToken(token string) {
	p.pollToken = token
}

// SetCIWebhookHandler sets the callback for processing CI webhooks from the relay.
func (p *RelayPoller) SetCIWebhookHandler(fn CIWebhookFunc) {
	p.onCIWebhook = fn
}

// Run starts the poll loop, blocking until ctx is cancelled.
func (p *RelayPoller) Run(ctx context.Context) {
	p.log.Info("relay poller started", "url", p.relayURL, "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Poll immediately on start.
	p.poll(ctx)

	for {
		select {
		case <-ticker.C:
			p.poll(ctx)
		case <-ctx.Done():
			p.log.Info("relay poller stopped")
			return
		}
	}
}

func (p *RelayPoller) poll(ctx context.Context) {
	url := fmt.Sprintf("%s/relay/poll?since=%d&limit=100", p.relayURL, p.lastID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		p.log.Error("relay poll: create request failed", "err", err)
		return
	}
	if p.pollToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.pollToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.log.Warn("relay poll: request failed", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		p.log.Warn("relay poll: bad status", "status", resp.StatusCode, "body", string(body))
		return
	}

	var result struct {
		Envelopes []RelayEnvelope `json:"envelopes"`
		Count     int             `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		p.log.Error("relay poll: decode failed", "err", err)
		return
	}

	if result.Count == 0 {
		return
	}

	p.log.Info("relay poll: received envelopes", "count", result.Count)

	var ackIDs []int64
	allOK := true
	for _, env := range result.Envelopes {
		// CI webhooks are stored with project_id=0. Process through CI handler if set.
		if env.ProjectID == 0 {
			if p.onCIWebhook != nil {
				if err := p.onCIWebhook(ctx, env.Payload); err != nil {
					p.log.Error("relay ci webhook processing failed", "id", env.ID, "err", err)
					allOK = false
					continue
				}
			}
			ackIDs = append(ackIDs, env.ID)
			continue
		}
		if err := p.ingest(ctx, env.ProjectID, env.PublicKey, env.Payload); err != nil {
			p.log.Error("relay ingest failed", "id", env.ID, "err", err)
			allOK = false
			continue
		}
		ackIDs = append(ackIDs, env.ID)
	}

	if len(ackIDs) > 0 {
		p.ack(ctx, ackIDs)
	}

	// Only advance lastID if every envelope in the batch was processed.
	// Otherwise, re-poll from the same position to retry failed envelopes.
	if allOK && len(result.Envelopes) > 0 {
		last := result.Envelopes[len(result.Envelopes)-1]
		if last.ID > p.lastID {
			p.lastID = last.ID
		}
	}
}

func (p *RelayPoller) ack(ctx context.Context, ids []int64) {
	body, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		p.log.Error("relay ack: marshal failed", "err", err)
		return
	}

	url := fmt.Sprintf("%s/relay/ack", p.relayURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		p.log.Error("relay ack: create request failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if p.pollToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.pollToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.log.Warn("relay ack: request failed", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		p.log.Warn("relay ack: bad status", "status", resp.StatusCode, "body", string(respBody))
		return
	}

	p.log.Info("relay ack: confirmed", "count", len(ids))
}
