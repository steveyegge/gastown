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
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Channel command flags
var (
	channelJSON        bool
	channelRetainCount int
	channelRetainHours int
)

var mailChannelCmd = &cobra.Command{
	Use:   "channel [name]",
	Short: "Manage and view beads-native channels",
	Long: `View and manage beads-native broadcast channels.

Without arguments, lists all channels.
With a channel name, shows messages from that channel.

Channels are pub/sub streams where messages are broadcast to subscribers.
Messages are retained according to the channel's retention policy.

Examples:
  gt mail channel              # List all channels
  gt mail channel alerts       # View messages from 'alerts' channel
  gt mail channel list         # Alias for listing channels
  gt mail channel show alerts  # Same as: gt mail channel alerts
  gt mail channel create alerts --retain-count=100
  gt mail channel delete alerts`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMailChannel,
}

var channelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all channels",
	Args:  cobra.NoArgs,
	RunE:  runChannelList,
}

var channelShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show channel messages",
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelShow,
}

var channelCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new channel",
	Long: `Create a new broadcast channel.

Retention policy:
  --retain-count=N  Keep only last N messages (0 = unlimited)
  --retain-hours=N  Delete messages older than N hours (0 = forever)`,
	Args: cobra.ExactArgs(1),
	RunE: runChannelCreate,
}

var channelDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a channel",
	Args:  cobra.ExactArgs(1),
	RunE:  runChannelDelete,
}

func init() {
	// List flags
	channelListCmd.Flags().BoolVar(&channelJSON, "json", false, "Output as JSON")

	// Show flags
	channelShowCmd.Flags().BoolVar(&channelJSON, "json", false, "Output as JSON")

	// Create flags
	channelCreateCmd.Flags().IntVar(&channelRetainCount, "retain-count", 0, "Number of messages to retain (0 = unlimited)")
	channelCreateCmd.Flags().IntVar(&channelRetainHours, "retain-hours", 0, "Hours to retain messages (0 = forever)")

	// Main channel command flags
	mailChannelCmd.Flags().BoolVar(&channelJSON, "json", false, "Output as JSON")

	// Add subcommands
	mailChannelCmd.AddCommand(channelListCmd)
	mailChannelCmd.AddCommand(channelShowCmd)
	mailChannelCmd.AddCommand(channelCreateCmd)
	mailChannelCmd.AddCommand(channelDeleteCmd)

	mailCmd.AddCommand(mailChannelCmd)
}

// runMailChannel handles the main channel command (list or show).
func runMailChannel(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return runChannelList(cmd, args)
	}
	return runChannelShow(cmd, args)
}

func runChannelList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	b := beads.New(townRoot)
	channels, err := b.ListChannelBeads()
	if err != nil {
		return fmt.Errorf("listing channels: %w", err)
	}

	if channelJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(channels)
	}

	if len(channels) == 0 {
		fmt.Println("No channels defined.")
		fmt.Println("\nCreate one with: gt mail channel create <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tRETENTION\tSTATUS\tCREATED BY")
	for name, fields := range channels {
		retention := "unlimited"
		if fields.RetentionCount > 0 {
			retention = fmt.Sprintf("%d msgs", fields.RetentionCount)
		} else if fields.RetentionHours > 0 {
			retention = fmt.Sprintf("%d hours", fields.RetentionHours)
		}
		status := fields.Status
		if status == "" {
			status = "active"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, retention, status, fields.CreatedBy)
	}
	return w.Flush()
}

func runChannelShow(cmd *cobra.Command, args []string) error {
	channelName := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	b := beads.New(townRoot)

	// Check if channel exists
	_, fields, err := b.GetChannelBead(channelName)
	if err != nil {
		return fmt.Errorf("getting channel: %w", err)
	}
	if fields == nil {
		return fmt.Errorf("channel not found: %s", channelName)
	}

	// Query messages for this channel
	messages, err := listChannelMessages(townRoot, channelName)
	if err != nil {
		return fmt.Errorf("listing channel messages: %w", err)
	}

	if channelJSON {
		if messages == nil {
			messages = []channelMessage{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	fmt.Printf("%s Channel: %s (%d messages)\n",
		style.Bold.Render("📡"), channelName, len(messages))
	if fields.RetentionCount > 0 {
		fmt.Printf("  Retention: %d messages\n", fields.RetentionCount)
	} else if fields.RetentionHours > 0 {
		fmt.Printf("  Retention: %d hours\n", fields.RetentionHours)
	}
	fmt.Println()

	if len(messages) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no messages)"))
		return nil
	}

	for _, msg := range messages {
		priorityMarker := ""
		if msg.Priority <= 1 {
			priorityMarker = " " + style.Bold.Render("!")
		}

		fmt.Printf("  %s %s%s\n", style.Bold.Render("●"), msg.Title, priorityMarker)
		fmt.Printf("    %s from %s\n",
			style.Dim.Render(msg.ID),
			msg.From)
		fmt.Printf("    %s\n",
			style.Dim.Render(msg.Created.Format("2006-01-02 15:04")))
		if msg.Body != "" {
			// Show first line as preview
			lines := strings.SplitN(msg.Body, "\n", 2)
			preview := lines[0]
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			fmt.Printf("    %s\n", style.Dim.Render(preview))
		}
	}

	return nil
}

func runChannelCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	if !isValidGroupName(name) { // Reuse group name validation
		return fmt.Errorf("invalid channel name %q: must be alphanumeric with dashes/underscores", name)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	createdBy := os.Getenv("BD_ACTOR")
	if createdBy == "" {
		createdBy = "unknown"
	}

	b := beads.New(townRoot)

	// Check if channel already exists
	existing, _, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("channel already exists: %s", name)
	}

	_, err = b.CreateChannelBead(name, nil, createdBy)
	if err != nil {
		return fmt.Errorf("creating channel: %w", err)
	}

	// Update retention settings if specified
	if channelRetainCount > 0 || channelRetainHours > 0 {
		if err := b.UpdateChannelRetention(name, channelRetainCount, channelRetainHours); err != nil {
			// Non-fatal: channel created but retention not set
			fmt.Printf("Warning: could not set retention: %v\n", err)
		}
	}

	fmt.Printf("Created channel %q", name)
	if channelRetainCount > 0 {
		fmt.Printf(" (retain %d messages)", channelRetainCount)
	} else if channelRetainHours > 0 {
		fmt.Printf(" (retain %d hours)", channelRetainHours)
	}
	fmt.Println()
	return nil
}

func runChannelDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	b := beads.New(townRoot)

	// Check if channel exists
	existing, _, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("channel not found: %s", name)
	}

	if err := b.DeleteChannelBead(name); err != nil {
		return fmt.Errorf("deleting channel: %w", err)
	}

	fmt.Printf("Deleted channel %q\n", name)
	return nil
}

// channelMessage represents a message in a channel.
type channelMessage struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Body     string    `json:"body,omitempty"`
	From     string    `json:"from"`
	Created  time.Time `json:"created"`
	Priority int       `json:"priority"`
}

// listChannelMessages lists messages from a beads-native channel.
func listChannelMessages(townRoot, channelName string) ([]channelMessage, error) {
	beadsDir := filepath.Join(townRoot, ".beads")

	// Query for messages with label channel:<name>
	args := []string{"list",
		"--type", "message",
		"--label", "channel:" + channelName,
		"--sort", "-created",
		"--limit", "0",
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

	var issues []struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Labels      []string  `json:"labels"`
		CreatedAt   time.Time `json:"created_at"`
		Priority    int       `json:"priority"`
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" || output == "[]" {
		return nil, nil
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	var messages []channelMessage
	for _, issue := range issues {
		msg := channelMessage{
			ID:       issue.ID,
			Title:    issue.Title,
			Body:     issue.Description,
			Created:  issue.CreatedAt,
			Priority: issue.Priority,
		}

		// Extract 'from' from labels
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "from:") {
				msg.From = strings.TrimPrefix(label, "from:")
				break
			}
		}

		messages = append(messages, msg)
	}

	// Sort by creation time (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Created.After(messages[j].Created)
	})

	return messages, nil
}
