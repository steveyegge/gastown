// Package slack provides an HTTP client for posting messages to Slack webhooks.
// It supports formatted decision messages using Block Kit, with retry logic
// and rate limiting awareness.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/util"
)

// Client is an HTTP client for posting to Slack webhooks.
type Client struct {
	webhookURL string
	httpClient *http.Client

	// Rate limiting: Slack allows ~1 message/second per webhook
	mu           sync.Mutex
	lastPostTime time.Time

	// Retry configuration
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		c.maxRetries = n
	}
}

// WithBackoff sets the initial and maximum backoff durations for retries.
func WithBackoff(initial, max time.Duration) Option {
	return func(c *Client) {
		c.initialBackoff = initial
		c.maxBackoff = max
	}
}

// NewClient creates a new Slack webhook client.
func NewClient(webhookURL string, opts ...Option) *Client {
	c := &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries:     3,
		initialBackoff: 1 * time.Second,
		maxBackoff:     30 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// DecisionOption represents an option in a decision.
type DecisionOption struct {
	Label       string
	Description string
	Recommended bool
}

// Decision represents a decision to be posted to Slack.
type Decision struct {
	ID          string
	Question    string
	Context     string
	Options     []DecisionOption
	Urgency     string // high, medium, low
	RequestedBy string
	Blockers    []string // Work IDs blocked by this decision
	ResolveURL  string   // Optional URL to resolve the decision
}

// PostDecision posts a formatted decision message to the Slack webhook.
func (c *Client) PostDecision(ctx context.Context, d *Decision) error {
	blocks := c.buildDecisionBlocks(d)
	return c.postBlocks(ctx, blocks)
}

// buildDecisionBlocks creates Block Kit blocks for a decision message.
func (c *Client) buildDecisionBlocks(d *Decision) []map[string]interface{} {
	urgencyEmoji := map[string]string{
		"high":   ":red_circle:",
		"medium": ":large_yellow_circle:",
		"low":    ":large_green_circle:",
	}
	emoji := urgencyEmoji[d.Urgency]
	if emoji == "" {
		emoji = ":white_circle:"
	}

	var blocks []map[string]interface{}

	// Header with urgency indicator
	blocks = append(blocks, map[string]interface{}{
		"type": "header",
		"text": map[string]interface{}{
			"type":  "plain_text",
			"text":  fmt.Sprintf("%s Decision Required", emoji),
			"emoji": true,
		},
	})

	// Main section with question and metadata
	questionText := fmt.Sprintf("*Question:* %s", d.Question)
	if d.RequestedBy != "" {
		questionText = fmt.Sprintf("*From:* %s\n%s", d.RequestedBy, questionText)
	}

	blocks = append(blocks, map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": questionText,
		},
	})

	// Context section if provided
	if d.Context != "" {
		// Truncate context if too long for Slack (max 3000 chars per text block)
		contextText := d.Context
		if len(contextText) > 2500 {
			contextText = contextText[:2497] + "..."
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Context:*\n%s", contextText),
			},
		})
	}

	// Urgency and ID fields
	semanticSlug := util.GenerateDecisionSlug(d.ID, d.Question)
	fields := []map[string]interface{}{
		{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Urgency:* %s %s", emoji, d.Urgency),
		},
		{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*ID:* `%s`", semanticSlug),
		},
	}

	blocks = append(blocks, map[string]interface{}{
		"type":   "section",
		"fields": fields,
	})

	// Divider before options
	blocks = append(blocks, map[string]interface{}{
		"type": "divider",
	})

	// Options section
	blocks = append(blocks, map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": "*Options:*",
		},
	})

	for i, opt := range d.Options {
		label := opt.Label
		if opt.Recommended {
			label = fmt.Sprintf(":star: %s _(recommended)_", label)
		}

		optText := fmt.Sprintf("*%d.* %s", i+1, label)
		if opt.Description != "" {
			optText += fmt.Sprintf("\n    %s", opt.Description)
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": optText,
			},
		})
	}

	// Blocked work section if any
	if len(d.Blockers) > 0 {
		blocks = append(blocks, map[string]interface{}{
			"type": "divider",
		})

		blockerText := "*Blocked work:*\n"
		for _, b := range d.Blockers {
			blockerText += fmt.Sprintf("- `%s`\n", b)
		}

		blocks = append(blocks, map[string]interface{}{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": blockerText,
				},
			},
		})
	}

	// Resolve link if provided
	if d.ResolveURL != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type": "button",
					"text": map[string]interface{}{
						"type":  "plain_text",
						"text":  "Resolve Decision",
						"emoji": true,
					},
					"url": d.ResolveURL,
				},
			},
		})
	}

	return blocks
}

// postBlocks sends blocks to the Slack webhook with retry logic.
func (c *Client) postBlocks(ctx context.Context, blocks []map[string]interface{}) error {
	payload := map[string]interface{}{
		"blocks": blocks,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	return c.postWithRetry(ctx, body)
}

// postWithRetry posts to the webhook with exponential backoff retry.
func (c *Client) postWithRetry(ctx context.Context, body []byte) error {
	var lastErr error
	backoff := c.initialBackoff

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Exponential backoff with cap
			backoff *= 2
			if backoff > c.maxBackoff {
				backoff = c.maxBackoff
			}
		}

		// Rate limiting: ensure at least 1 second between posts
		c.enforceRateLimit()

		err := c.doPost(ctx, body)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// enforceRateLimit ensures we don't exceed Slack's rate limits.
func (c *Client) enforceRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Slack webhooks allow approximately 1 message per second
	minInterval := time.Second

	elapsed := time.Since(c.lastPostTime)
	if elapsed < minInterval {
		time.Sleep(minInterval - elapsed)
	}

	c.lastPostTime = time.Now()
}

// doPost performs the actual HTTP POST to the webhook.
func (c *Client) doPost(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &RetryableError{Err: fmt.Errorf("post webhook: %w", err)}
	}
	defer resp.Body.Close()

	// Read response body for error messages
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		return &RetryableError{
			Err:       fmt.Errorf("rate limited (429): %s", string(respBody)),
			RateLimit: true,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return &RetryableError{Err: fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))}
	default:
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(respBody))
	}
}

// RetryableError indicates an error that may be retried.
type RetryableError struct {
	Err       error
	RateLimit bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// isRetryableError checks if an error should trigger a retry.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*RetryableError)
	return ok
}
