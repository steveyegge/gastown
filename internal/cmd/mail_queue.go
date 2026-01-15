package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// runMailClaim claims the oldest unclaimed message from a work queue.
// If a queue name is provided, claims from that specific queue.
// If no queue name is provided, claims from any queue the caller is eligible for.
func runMailClaim(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get caller identity
	caller := detectSender()
	beadsDir := beads.ResolveBeadsDir(townRoot)
	bd := beads.NewWithBeadsDir(townRoot, beadsDir)

	var queueName string
	var queueFields *beads.QueueFields

	if len(args) > 0 {
		// Specific queue requested
		queueName = args[0]

		// Look up the queue bead
		queueID := beads.QueueBeadID(queueName, true) // Try town-level first
		issue, fields, err := bd.GetQueueBead(queueID)
		if err != nil {
			return fmt.Errorf("looking up queue: %w", err)
		}
		if issue == nil {
			// Try rig-level
			queueID = beads.QueueBeadID(queueName, false)
			issue, fields, err = bd.GetQueueBead(queueID)
			if err != nil {
				return fmt.Errorf("looking up queue: %w", err)
			}
			if issue == nil {
				return fmt.Errorf("unknown queue: %s", queueName)
			}
		}
		queueFields = fields

		// Check if caller is eligible
		if !beads.MatchClaimPattern(queueFields.ClaimPattern, caller) {
			return fmt.Errorf("not eligible to claim from queue %s (caller: %s, pattern: %s)",
				queueName, caller, queueFields.ClaimPattern)
		}
	} else {
		// No queue specified - find any queue the caller can claim from
		eligibleIssues, eligibleFields, err := bd.FindEligibleQueues(caller)
		if err != nil {
			return fmt.Errorf("finding eligible queues: %w", err)
		}
		if len(eligibleIssues) == 0 {
			fmt.Printf("%s No queues available for claiming (caller: %s)\n",
				style.Dim.Render("○"), caller)
			return nil
		}

		// Use the first eligible queue
		queueFields = eligibleFields[0]
		queueName = queueFields.Name
		if queueName == "" {
			// Fallback to ID-based name
			queueName = eligibleIssues[0].ID
		}
	}

	// List unclaimed messages in the queue
	// Queue messages have queue:<name> label and no claimed-by label
	messages, err := listUnclaimedQueueMessages(beadsDir, queueName)
	if err != nil {
		return fmt.Errorf("listing queue messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("%s No messages to claim in queue %s\n", style.Dim.Render("○"), queueName)
		return nil
	}

	// Pick the oldest unclaimed message (first in list, sorted by created)
	oldest := messages[0]

	// Claim the message: add claimed-by and claimed-at labels
	if err := claimQueueMessage(beadsDir, oldest.ID, caller); err != nil {
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
	ClaimedBy   string
	ClaimedAt   *time.Time
}

// listUnclaimedQueueMessages lists unclaimed messages in a queue.
// Unclaimed messages have queue:<name> label but no claimed-by label.
func listUnclaimedQueueMessages(beadsDir, queueName string) ([]queueMessage, error) {
	// Use bd list to find messages with queue:<name> label and status=open
	args := []string{"list",
		"--label", "queue:" + queueName,
		"--status", "open",
		"--type", "message",
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

	// Convert to queueMessage, filtering out already claimed messages
	var messages []queueMessage
	for _, issue := range issues {
		msg := queueMessage{
			ID:          issue.ID,
			Title:       issue.Title,
			Description: issue.Description,
			Created:     issue.CreatedAt,
			Priority:    issue.Priority,
		}

		// Extract labels
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "from:") {
				msg.From = strings.TrimPrefix(label, "from:")
			} else if strings.HasPrefix(label, "claimed-by:") {
				msg.ClaimedBy = strings.TrimPrefix(label, "claimed-by:")
			} else if strings.HasPrefix(label, "claimed-at:") {
				ts := strings.TrimPrefix(label, "claimed-at:")
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					msg.ClaimedAt = &t
				}
			}
		}

		// Only include unclaimed messages
		if msg.ClaimedBy == "" {
			messages = append(messages, msg)
		}
	}

	// Sort by created time (oldest first) for FIFO ordering
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Created.Before(messages[j].Created)
	})

	return messages, nil
}

// claimQueueMessage claims a message by adding claimed-by and claimed-at labels.
func claimQueueMessage(beadsDir, messageID, claimant string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	args := []string{"label", "add", messageID,
		"claimed-by:" + claimant,
		"claimed-at:" + now,
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

	beadsDir := beads.ResolveBeadsDir(townRoot)

	// Get caller identity
	caller := detectSender()

	// Get message details to verify ownership and find queue
	msgInfo, err := getQueueMessageInfo(beadsDir, messageID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Verify message exists and is a queue message
	if msgInfo.QueueName == "" {
		return fmt.Errorf("message %s is not a queue message (no queue label)", messageID)
	}

	// Verify caller is the one who claimed it
	if msgInfo.ClaimedBy == "" {
		return fmt.Errorf("message %s is not claimed", messageID)
	}
	if msgInfo.ClaimedBy != caller {
		return fmt.Errorf("message %s was claimed by %s, not %s", messageID, msgInfo.ClaimedBy, caller)
	}

	// Release the message: remove claimed-by and claimed-at labels
	if err := releaseQueueMessage(beadsDir, messageID, caller); err != nil {
		return fmt.Errorf("releasing message: %w", err)
	}

	fmt.Printf("%s Released message back to queue %s\n", style.Bold.Render("✓"), msgInfo.QueueName)
	fmt.Printf("  ID: %s\n", messageID)
	fmt.Printf("  Subject: %s\n", msgInfo.Title)

	return nil
}

// queueMessageInfo holds details about a queue message.
type queueMessageInfo struct {
	ID        string
	Title     string
	QueueName string
	ClaimedBy string
	ClaimedAt *time.Time
	Status    string
}

// getQueueMessageInfo retrieves information about a queue message.
func getQueueMessageInfo(beadsDir, messageID string) (*queueMessageInfo, error) {
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
	info := &queueMessageInfo{
		ID:     issue.ID,
		Title:  issue.Title,
		Status: issue.Status,
	}

	// Extract fields from labels
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "queue:") {
			info.QueueName = strings.TrimPrefix(label, "queue:")
		} else if strings.HasPrefix(label, "claimed-by:") {
			info.ClaimedBy = strings.TrimPrefix(label, "claimed-by:")
		} else if strings.HasPrefix(label, "claimed-at:") {
			ts := strings.TrimPrefix(label, "claimed-at:")
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				info.ClaimedAt = &t
			}
		}
	}

	return info, nil
}

// releaseQueueMessage releases a claimed message by removing claim labels.
func releaseQueueMessage(beadsDir, messageID, actor string) error {
	// Get current message info to find the exact claim labels
	info, err := getQueueMessageInfo(beadsDir, messageID)
	if err != nil {
		return err
	}

	// Remove claimed-by label
	if info.ClaimedBy != "" {
		args := []string{"label", "remove", messageID, "claimed-by:" + info.ClaimedBy}
		cmd := exec.Command("bd", args...)
		cmd.Env = append(os.Environ(),
			"BEADS_DIR="+beadsDir,
			"BD_ACTOR="+actor,
		)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" && !strings.Contains(errMsg, "does not have label") {
				return fmt.Errorf("%s", errMsg)
			}
		}
	}

	// Remove claimed-at label if present
	if info.ClaimedAt != nil {
		claimedAtStr := info.ClaimedAt.Format(time.RFC3339)
		args := []string{"label", "remove", messageID, "claimed-at:" + claimedAtStr}
		cmd := exec.Command("bd", args...)
		cmd.Env = append(os.Environ(),
			"BEADS_DIR="+beadsDir,
			"BD_ACTOR="+actor,
		)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" && !strings.Contains(errMsg, "does not have label") {
				return fmt.Errorf("%s", errMsg)
			}
		}
	}

	return nil
}
