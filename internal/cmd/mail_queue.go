package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// runMailClaim claims the oldest unclaimed message from a work queue.
func runMailClaim(cmd *cobra.Command, args []string) error {
	queueName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load queue config from messaging.json
	configPath := config.MessagingConfigPath(townRoot)
	cfg, err := config.LoadMessagingConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading messaging config: %w", err)
	}

	queueCfg, ok := cfg.Queues[queueName]
	if !ok || queueCfg == nil {
		return fmt.Errorf("unknown queue: %s", queueName)
	}

	// Get caller identity
	caller := detectSender()

	// Check if caller is eligible (matches any pattern in workers list)
	if !isEligibleWorker(caller, queueCfg.Workers) {
		return fmt.Errorf("not eligible to claim from queue %s (caller: %s, workers: %v)",
			queueName, caller, queueCfg.Workers)
	}

	// List unclaimed messages in the queue
	// Queue messages have assignee=queue:<name> and status=open
	queueAssignee := "queue:" + queueName
	messages, err := listQueueMessages(townRoot, queueAssignee)
	if err != nil {
		return fmt.Errorf("listing queue messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("%s No messages to claim in queue %s\n", style.Dim.Render("○"), queueName)
		return nil
	}

	// Pick the oldest unclaimed message (first in list, sorted by created)
	oldest := messages[0]

	// Claim the message: set assignee to caller and status to in_progress
	if err := claimMessage(townRoot, oldest.ID, caller); err != nil {
		return fmt.Errorf("claiming message: %w", err)
	}

	// Print claimed message details
	fmt.Printf("%s Claimed message from queue %s\n", style.Bold.Render("✓"), queueName)
	fmt.Printf("  ID: %s\n", oldest.ID)
	fmt.Printf("  Subject: %s\n", oldest.Title)
	if oldest.Description != "" {
		// Show first line of description
		lines := strings.SplitN(oldest.Description, "\n", 2)
		preview := lines[0]
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		fmt.Printf("  Preview: %s\n", style.Dim.Render(preview))
	}
	fmt.Printf("  From: %s\n", oldest.From)
	fmt.Printf("  Created: %s\n", oldest.Created.Format("2006-01-02 15:04"))

	return nil
}

// queueMessage represents a message in a queue.
type queueMessage struct {
	ID          string
	Title       string
	Description string
	From        string
	Created     time.Time
	Priority    int
}

// isEligibleWorker checks if the caller matches any pattern in the workers list.
// Patterns support wildcards: "gastown/polecats/*" matches "gastown/polecats/capable".
func isEligibleWorker(caller string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchWorkerPattern(pattern, caller) {
			return true
		}
	}
	return false
}

// matchWorkerPattern checks if caller matches the pattern.
// Supports simple wildcards: * matches a single path segment (no slashes).
func matchWorkerPattern(pattern, caller string) bool {
	// Handle exact match
	if pattern == caller {
		return true
	}

	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		// Convert to simple glob matching
		// "gastown/polecats/*" should match "gastown/polecats/capable"
		// but NOT "gastown/polecats/sub/capable"
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			if strings.HasPrefix(caller, prefix) && strings.HasSuffix(caller, suffix) {
				// Check that the middle part doesn't contain path separators
				middle := caller[len(prefix) : len(caller)-len(suffix)]
				if !strings.Contains(middle, "/") {
					return true
				}
			}
		}
	}

	return false
}

// listQueueMessages lists unclaimed messages in a queue.
func listQueueMessages(townRoot, queueAssignee string) ([]queueMessage, error) {
	// Use bd list to find messages with assignee=queue:<name> and status=open
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"list",
		"--assignee", queueAssignee,
		"--status", "open",
		"--type", "message",
		"--sort", "created",
		"--limit", "0", // No limit
		"--json",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// Parse JSON output
	var issues []struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Labels      []string  `json:"labels"`
		CreatedAt   time.Time `json:"created_at"`
		Priority    int       `json:"priority"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		// If no messages, bd might output empty or error
		if strings.TrimSpace(stdout.String()) == "" || strings.TrimSpace(stdout.String()) == "[]" {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	// Convert to queueMessage, extracting 'from' from labels
	var messages []queueMessage
	for _, issue := range issues {
		msg := queueMessage{
			ID:          issue.ID,
			Title:       issue.Title,
			Description: issue.Description,
			Created:     issue.CreatedAt,
			Priority:    issue.Priority,
		}

		// Extract 'from' from labels (format: "from:address")
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "from:") {
				msg.From = strings.TrimPrefix(label, "from:")
				break
			}
		}

		messages = append(messages, msg)
	}

	// Sort by created time (oldest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Created.Before(messages[j].Created)
	})

	return messages, nil
}

// claimMessage claims a message by setting assignee and status.
func claimMessage(townRoot, messageID, claimant string) error {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"update", messageID,
		"--assignee", claimant,
		"--status", "in_progress",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(),
		"BEADS_DIR="+beadsDir,
		"BD_ACTOR="+claimant,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}

// runMailRelease releases a claimed queue message back to its queue.
func runMailRelease(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get caller identity
	caller := detectSender()

	// Get message details to verify ownership and find queue
	msgInfo, err := getMessageInfo(townRoot, messageID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Verify message exists and is a queue message
	if msgInfo.QueueName == "" {
		return fmt.Errorf("message %s is not a queue message (no queue label)", messageID)
	}

	// Verify caller is the one who claimed it
	if msgInfo.Assignee != caller {
		if strings.HasPrefix(msgInfo.Assignee, "queue:") {
			return fmt.Errorf("message %s is not claimed (still in queue)", messageID)
		}
		return fmt.Errorf("message %s was claimed by %s, not %s", messageID, msgInfo.Assignee, caller)
	}

	// Release the message: set assignee back to queue and status to open
	queueAssignee := "queue:" + msgInfo.QueueName
	if err := releaseMessage(townRoot, messageID, queueAssignee, caller); err != nil {
		return fmt.Errorf("releasing message: %w", err)
	}

	fmt.Printf("%s Released message back to queue %s\n", style.Bold.Render("✓"), msgInfo.QueueName)
	fmt.Printf("  ID: %s\n", messageID)
	fmt.Printf("  Subject: %s\n", msgInfo.Title)

	return nil
}

// messageInfo holds details about a queue message.
type messageInfo struct {
	ID        string
	Title     string
	Assignee  string
	QueueName string
	Status    string
}

// getMessageInfo retrieves information about a message.
func getMessageInfo(townRoot, messageID string) (*messageInfo, error) {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"show", messageID, "--json"}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "not found") {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// Parse JSON output - bd show --json returns an array
	var issues []struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Assignee string   `json:"assignee"`
		Labels   []string `json:"labels"`
		Status   string   `json:"status"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing message: %w", err)
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}

	issue := issues[0]
	info := &messageInfo{
		ID:       issue.ID,
		Title:    issue.Title,
		Assignee: issue.Assignee,
		Status:   issue.Status,
	}

	// Extract queue name from labels (format: "queue:<name>")
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "queue:") {
			info.QueueName = strings.TrimPrefix(label, "queue:")
			break
		}
	}

	return info, nil
}

// releaseMessage releases a claimed message back to its queue.
func releaseMessage(townRoot, messageID, queueAssignee, actor string) error {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"update", messageID,
		"--assignee", queueAssignee,
		"--status", "open",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(),
		"BEADS_DIR="+beadsDir,
		"BD_ACTOR="+actor,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}
