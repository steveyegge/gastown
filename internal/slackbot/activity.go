// Package slackbot implements a Slack bot for Gas Town decision management.
package slackbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// ActivityEntry represents a single activity event from an agent.
type ActivityEntry struct {
	Timestamp time.Time
	Type      string // "commit", "event", "session"
	Message   string
}

// handlePeekButton handles the Peek button click to show agent activity in thread.
func (b *Bot) handlePeekButton(callback slack.InteractionCallback, decisionID string) {
	// Get the message info from our tracked messages
	b.decisionMessagesMu.RLock()
	msgInfo, ok := b.decisionMessages[decisionID]
	b.decisionMessagesMu.RUnlock()

	if !ok {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"Could not find message info for this decision.")
		return
	}

	// Look up the decision to get the agent
	ctx := context.Background()
	decision, err := b.rpcClient.GetDecision(ctx, decisionID)
	if err != nil || decision == nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Could not find decision: %v", err))
		return
	}

	agent := decision.RequestedBy
	if agent == "" {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			"No agent associated with this decision.")
		return
	}

	// Get agent activity
	activities := b.getAgentActivity(agent, 10)

	// Format as code block
	blocks := formatActivityBlocks(agent, activities)

	// Post to thread
	_, _, err = b.client.PostMessage(
		msgInfo.channelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionTS(msgInfo.timestamp),
	)
	if err != nil {
		b.postEphemeral(callback.Channel.ID, callback.User.ID,
			fmt.Sprintf("Error posting activity: %v", err))
	}
}

// getAgentActivity fetches recent activity for an agent from multiple sources.
func (b *Bot) getAgentActivity(agent string, limit int) []ActivityEntry {
	var activities []ActivityEntry

	// Source 1: Git log for recent commits by this agent
	gitActivities := b.getGitActivity(agent, limit)
	activities = append(activities, gitActivities...)

	// Source 2: Events from ~/.events.jsonl
	eventActivities := b.getEventActivity(agent, limit)
	activities = append(activities, eventActivities...)

	// Source 3: gt peek output (if available)
	peekActivities := b.getPeekActivity(agent)
	activities = append(activities, peekActivities...)

	// Sort by timestamp descending (most recent first)
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].Timestamp.After(activities[j].Timestamp)
	})

	// Limit results
	if len(activities) > limit {
		activities = activities[:limit]
	}

	return activities
}

// getGitActivity gets recent git commits mentioning the agent.
func (b *Bot) getGitActivity(agent string, limit int) []ActivityEntry {
	var activities []ActivityEntry

	// Try to run git log
	cmd := exec.Command("git", "log",
		"--oneline",
		"-n", fmt.Sprintf("%d", limit),
		"--format=%H|%ai|%s",
		"--all",
	)
	cmd.Dir = b.townRoot

	output, err := cmd.Output()
	if err != nil {
		return activities
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		// Check if commit message mentions the agent (in parens like (gt-xxx))
		subject := parts[2]
		if !strings.Contains(strings.ToLower(subject), strings.ToLower(extractAgentShortName(agent))) {
			continue
		}

		timestamp, err := time.Parse("2006-01-02 15:04:05 -0700", parts[1])
		if err != nil {
			timestamp = time.Now()
		}

		activities = append(activities, ActivityEntry{
			Timestamp: timestamp,
			Type:      "commit",
			Message:   subject,
		})
	}

	return activities
}

// getEventActivity reads events from ~/.events.jsonl for the agent.
func (b *Bot) getEventActivity(agent string, limit int) []ActivityEntry {
	var activities []ActivityEntry

	eventsPath := filepath.Join(os.Getenv("HOME"), ".events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		return activities
	}
	defer file.Close()

	// Read last N lines (tail -like behavior)
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Process from end, looking for matching events
	shortName := extractAgentShortName(agent)
	for i := len(lines) - 1; i >= 0 && len(activities) < limit; i-- {
		line := lines[i]

		var event struct {
			Timestamp string `json:"timestamp"`
			Actor     string `json:"actor"`
			Type      string `json:"type"`
			Message   string `json:"message"`
			Action    string `json:"action"`
		}

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Check if this event is from our agent
		if !strings.Contains(strings.ToLower(event.Actor), strings.ToLower(shortName)) {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil {
			timestamp = time.Now()
		}

		msg := event.Message
		if msg == "" {
			msg = event.Action
		}
		if msg == "" {
			msg = event.Type
		}

		activities = append(activities, ActivityEntry{
			Timestamp: timestamp,
			Type:      "event",
			Message:   msg,
		})
	}

	return activities
}

// getPeekActivity runs gt peek to get live session info.
func (b *Bot) getPeekActivity(agent string) []ActivityEntry {
	var activities []ActivityEntry

	cmd := exec.Command("gt", "peek", agent)
	cmd.Dir = b.townRoot

	output, err := cmd.Output()
	if err != nil {
		return activities
	}

	// Parse gt peek output - it returns formatted activity info
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "─") {
			continue
		}

		// gt peek output format typically includes timestamps like [HH:MM]
		if strings.HasPrefix(line, "[") {
			activities = append(activities, ActivityEntry{
				Timestamp: time.Now(), // Use current time as approximation
				Type:      "session",
				Message:   line,
			})
		}
	}

	return activities
}

// extractAgentShortName extracts the short name from an agent path like "gastown/crew/decisions".
func extractAgentShortName(agent string) string {
	parts := strings.Split(agent, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return agent
}

// formatActivityBlocks formats activity entries as Slack blocks.
func formatActivityBlocks(agent string, activities []ActivityEntry) []slack.Block {
	blocks := []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text",
				fmt.Sprintf("👁️ Activity: %s", extractAgentShortName(agent)),
				false, false),
		),
	}

	if len(activities) == 0 {
		blocks = append(blocks,
			slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					"_No recent activity found_",
					false, false),
				nil, nil),
		)
		return blocks
	}

	// Build activity text as code block
	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString("─────────────────────────────────────────\n")

	for _, a := range activities {
		timeStr := a.Timestamp.Format("15:04")
		typeIcon := "•"
		switch a.Type {
		case "commit":
			typeIcon = "📝"
		case "event":
			typeIcon = "⚡"
		case "session":
			typeIcon = "💬"
		}

		// Truncate long messages
		msg := a.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}

		sb.WriteString(fmt.Sprintf("[%s] %s %s\n", timeStr, typeIcon, msg))
	}

	sb.WriteString("─────────────────────────────────────────\n")
	sb.WriteString("```")

	blocks = append(blocks,
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", sb.String(), false, false),
			nil, nil),
	)

	return blocks
}
