package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/workspace"
)

// SemanticConflict represents a detected conflict between Polecat modifications.
type SemanticConflict struct {
	BeadID  string                `json:"bead_id"`
	Field   string                `json:"field"`
	Changes []SemanticFieldChange `json:"changes"`
}

// SemanticFieldChange represents a single modification to a bead field.
type SemanticFieldChange struct {
	Polecat    string  `json:"polecat"`
	OldValue   string  `json:"old_value"`
	NewValue   string  `json:"new_value"`
	Confidence float64 `json:"confidence,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
	CommitSHA  string  `json:"commit_sha,omitempty"`
}

// ConflictDetectionResult is the output of the detect command.
type ConflictDetectionResult struct {
	MR        string             `json:"mr"`
	Branch    string             `json:"branch"`
	Target    string             `json:"target"`
	Conflicts []SemanticConflict `json:"conflicts"`
	Detected  time.Time          `json:"detected"`
}

var semanticConflictCmd = &cobra.Command{
	Use:     "semantic-conflict",
	GroupID: GroupWork,
	Short:   "Detect and handle semantic conflicts in MR bead modifications",
	Long: `Semantic conflict detection plugin for Gas Town Refinery.

Detects when multiple Polecats modify the same bead field with different values
(semantic conflicts) and escalates to Mayor for decision rather than using
automatic resolution (LWW).

This command is typically invoked by the mol-semantic-conflict-detector plugin
molecule during Refinery MR processing.

Subcommands:
  detect    - Analyze commits for conflicting bead modifications
  escalate  - Send conflict details to Mayor for decision
  await     - Wait for Mayor's resolution
  apply     - Apply resolved values to beads`,
}

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect semantic conflicts in MR branch",
	Long: `Analyze commits in an MR branch to detect semantic conflicts.

Looks for BEAD_CHANGES metadata blocks in commit messages and identifies
conflicts where:
  - Same bead field modified by different Polecats
  - Different values set for the field
  - Field is in the escalate-fields list

Output is JSON with detected conflicts.

Example:
  gt semantic-conflict detect --branch polecat/toast/gt-abc --target main`,
	RunE: runSemanticConflictDetect,
}

var escalateCmd2 = &cobra.Command{
	Use:   "escalate",
	Short: "Escalate conflicts to Mayor for decision",
	Long: `Send escalation mail to Mayor with conflict details.

Reads conflicts from stdin or --conflicts file and sends formatted
escalation mail to Mayor with confidence scores and reasoning.

Example:
  gt semantic-conflict escalate --conflicts conflicts.json --mr gt-abc123`,
	RunE: runSemanticConflictEscalate,
}

var awaitCmd = &cobra.Command{
	Use:   "await",
	Short: "Wait for Mayor's resolution",
	Long: `Block until Mayor responds with conflict resolution.

Polls for mail with subject "SEMANTIC_CONFLICT_RESOLVED <mr-id>" and
outputs the resolution when received.

Example:
  gt semantic-conflict await --mr gt-abc123 --timeout 1h`,
	RunE: runSemanticConflictAwait,
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply Mayor's resolution to beads",
	Long: `Apply resolved values to affected beads.

Reads resolution from stdin or --resolution file and updates each
bead field with the resolved value.

Example:
  gt semantic-conflict apply --resolution resolution.json`,
	RunE: runSemanticConflictApply,
}

var (
	scBranch        string
	scTarget        string
	scFields        string
	scConflictsFile string
	scMRID          string
	scTimeout       string
	scResolutionFile string
	scOutput        string
)

func init() {
	// detect flags
	detectCmd.Flags().StringVar(&scBranch, "branch", "", "MR branch to analyze (required)")
	detectCmd.Flags().StringVar(&scTarget, "target", "main", "Target branch")
	detectCmd.Flags().StringVar(&scFields, "fields", "priority,assignee", "Comma-separated fields to check")
	detectCmd.Flags().StringVarP(&scOutput, "output", "o", "", "Output file (default: stdout)")
	_ = detectCmd.MarkFlagRequired("branch")

	// escalate flags
	escalateCmd2.Flags().StringVar(&scConflictsFile, "conflicts", "", "Conflicts JSON file (default: stdin)")
	escalateCmd2.Flags().StringVar(&scMRID, "mr", "", "MR ID (required)")
	_ = escalateCmd2.MarkFlagRequired("mr")

	// await flags
	awaitCmd.Flags().StringVar(&scMRID, "mr", "", "MR ID (required)")
	awaitCmd.Flags().StringVar(&scTimeout, "timeout", "1h", "Timeout waiting for response")
	_ = awaitCmd.MarkFlagRequired("mr")

	// apply flags
	applyCmd.Flags().StringVar(&scResolutionFile, "resolution", "", "Resolution JSON file (default: stdin)")
	applyCmd.Flags().StringVar(&scMRID, "mr", "", "MR ID")

	// Add subcommands
	semanticConflictCmd.AddCommand(detectCmd)
	semanticConflictCmd.AddCommand(escalateCmd2)
	semanticConflictCmd.AddCommand(awaitCmd)
	semanticConflictCmd.AddCommand(applyCmd)

	rootCmd.AddCommand(semanticConflictCmd)
}

func runSemanticConflictDetect(cmd *cobra.Command, args []string) error {
	// Parse fields to check
	fieldsToCheck := strings.Split(scFields, ",")
	for i := range fieldsToCheck {
		fieldsToCheck[i] = strings.TrimSpace(fieldsToCheck[i])
	}

	// Get commits in branch range
	commits, err := getCommitsInRange(scTarget, scBranch)
	if err != nil {
		return fmt.Errorf("getting commits: %w", err)
	}

	// Extract bead changes from commits
	allChanges := []beadChangeWithMeta{}
	for _, commit := range commits {
		changes := parseBeadChangesFromCommit(commit)
		allChanges = append(allChanges, changes...)
	}

	// Group by bead:field
	grouped := groupChangesByBeadField(allChanges)

	// Detect conflicts
	conflicts := []SemanticConflict{}
	for key, changes := range grouped {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		beadID, field := parts[0], parts[1]

		// Check if field should be escalated
		shouldCheck := false
		for _, f := range fieldsToCheck {
			if f == field {
				shouldCheck = true
				break
			}
		}
		if !shouldCheck {
			continue
		}

		// Check for conflict
		if isConflict(changes) {
			scChanges := []SemanticFieldChange{}
			for _, ch := range changes {
				scChanges = append(scChanges, SemanticFieldChange{
					Polecat:    ch.Polecat,
					OldValue:   ch.OldValue,
					NewValue:   ch.NewValue,
					Confidence: ch.Confidence,
					Reasoning:  ch.Reasoning,
					CommitSHA:  ch.CommitSHA,
				})
			}
			conflicts = append(conflicts, SemanticConflict{
				BeadID:  beadID,
				Field:   field,
				Changes: scChanges,
			})
		}
	}

	// Build result
	result := ConflictDetectionResult{
		MR:        scMRID,
		Branch:    scBranch,
		Target:    scTarget,
		Conflicts: conflicts,
		Detected:  time.Now(),
	}

	// Output
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	if scOutput != "" {
		if err := os.WriteFile(scOutput, jsonData, 0644); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		fmt.Printf("Wrote %d conflict(s) to %s\n", len(conflicts), scOutput)
	} else {
		fmt.Println(string(jsonData))
	}

	// Exit with code 1 if conflicts found (for scripting)
	if len(conflicts) > 0 {
		return fmt.Errorf("detected %d semantic conflict(s)", len(conflicts))
	}

	return nil
}

func runSemanticConflictEscalate(cmd *cobra.Command, args []string) error {
	// Read conflicts
	var data []byte
	var err error
	if scConflictsFile != "" {
		data, err = os.ReadFile(scConflictsFile)
	} else {
		data, err = readStdin()
	}
	if err != nil {
		return fmt.Errorf("reading conflicts: %w", err)
	}

	var result ConflictDetectionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parsing conflicts: %w", err)
	}

	if len(result.Conflicts) == 0 {
		fmt.Println("No conflicts to escalate")
		return nil
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Build escalation mail
	subject := fmt.Sprintf("SEMANTIC_CONFLICT_ESCALATED %s", scMRID)
	body := buildEscalationMailBody(scMRID, result.Conflicts)

	// Send to Mayor
	router := mail.NewRouter(townRoot)
	msg := mail.NewMessage("refinery", "mayor/", subject, body)
	msg.Priority = mail.PriorityHigh
	msg.Type = mail.TypeTask
	msg.ThreadID = "semantic-conflict-" + scMRID

	if err := router.Send(msg); err != nil {
		return fmt.Errorf("sending escalation mail: %w", err)
	}

	fmt.Printf("Escalated %d conflict(s) to Mayor\n", len(result.Conflicts))
	fmt.Printf("Mail ID: %s\n", msg.ID)
	fmt.Printf("Thread: %s\n", msg.ThreadID)

	return nil
}

func runSemanticConflictAwait(cmd *cobra.Command, args []string) error {
	timeout, err := time.ParseDuration(scTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	expectedSubject := fmt.Sprintf("SEMANTIC_CONFLICT_RESOLVED %s", scMRID)
	deadline := time.Now().Add(timeout)

	fmt.Printf("Waiting for resolution (timeout: %s)...\n", scTimeout)

	// Poll for resolution mail
	for time.Now().Before(deadline) {
		// Get mailbox for refinery
		router := mail.NewRouter(townRoot)
		mailbox, err := router.GetMailbox("refinery")
		if err != nil {
			time.Sleep(30 * time.Second)
			continue
		}

		// Check unread messages
		messages, err := mailbox.ListUnread()
		if err != nil {
			time.Sleep(30 * time.Second)
			continue
		}

		for _, msg := range messages {
			if strings.Contains(msg.Subject, expectedSubject) {
				fmt.Printf("Resolution received from: %s\n", msg.From)
				fmt.Println(msg.Body)
				return nil
			}
		}

		time.Sleep(30 * time.Second)
	}

	return fmt.Errorf("timeout waiting for Mayor resolution after %s", scTimeout)
}

func runSemanticConflictApply(cmd *cobra.Command, args []string) error {
	// Read resolution
	var data []byte
	var err error
	if scResolutionFile != "" {
		data, err = os.ReadFile(scResolutionFile)
	} else {
		data, err = readStdin()
	}
	if err != nil {
		return fmt.Errorf("reading resolution: %w", err)
	}

	var resolution struct {
		Resolutions map[string]string `json:"resolutions"`
		Reasoning   string            `json:"reasoning"`
	}
	if err := json.Unmarshal(data, &resolution); err != nil {
		return fmt.Errorf("parsing resolution: %w", err)
	}

	// Apply each resolution
	for key, value := range resolution.Resolutions {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			fmt.Printf("Warning: invalid key format: %s\n", key)
			continue
		}
		beadID, field := parts[0], parts[1]

		// Use bd to update the bead
		cmd := exec.Command("bd", "update", beadID, fmt.Sprintf("--%s=%s", field, value))
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: failed to update %s.%s: %v\n%s\n", beadID, field, err, out)
		} else {
			fmt.Printf("Applied: %s.%s = %s\n", beadID, field, value)
		}
	}

	if resolution.Reasoning != "" {
		fmt.Printf("\nMayor's reasoning: %s\n", resolution.Reasoning)
	}

	return nil
}

// Helper types and functions

type beadChangeWithMeta struct {
	BeadID     string
	Field      string
	Polecat    string
	OldValue   string
	NewValue   string
	Confidence float64
	Reasoning  string
	CommitSHA  string
}

type commitInfo struct {
	SHA     string
	Message string
}

func getCommitsInRange(target, branch string) ([]commitInfo, error) {
	// Use git log to get commits
	cmd := exec.Command("git", "log", "--format=%H|||%B|||END|||",
		fmt.Sprintf("origin/%s..origin/%s", target, branch))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	commits := []commitInfo{}
	entries := strings.Split(string(out), "|||END|||")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "|||", 2)
		if len(parts) == 2 {
			commits = append(commits, commitInfo{
				SHA:     strings.TrimSpace(parts[0]),
				Message: strings.TrimSpace(parts[1]),
			})
		}
	}
	return commits, nil
}

func parseBeadChangesFromCommit(commit commitInfo) []beadChangeWithMeta {
	changes := []beadChangeWithMeta{}

	// Look for BEAD_CHANGES: block in commit message
	idx := strings.Index(commit.Message, "BEAD_CHANGES:")
	if idx == -1 {
		return changes
	}

	jsonStart := idx + len("BEAD_CHANGES:")
	jsonStr := strings.TrimSpace(commit.Message[jsonStart:])

	// Try to parse JSON
	var changeData struct {
		BeadID  string `json:"bead_id"`
		Polecat string `json:"polecat"`
		Changes []struct {
			Field      string  `json:"field"`
			OldValue   string  `json:"old_value"`
			NewValue   string  `json:"new_value"`
			Confidence float64 `json:"confidence"`
			Reasoning  string  `json:"reasoning"`
		} `json:"changes"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &changeData); err == nil {
		for _, ch := range changeData.Changes {
			changes = append(changes, beadChangeWithMeta{
				BeadID:     changeData.BeadID,
				Field:      ch.Field,
				Polecat:    changeData.Polecat,
				OldValue:   ch.OldValue,
				NewValue:   ch.NewValue,
				Confidence: ch.Confidence,
				Reasoning:  ch.Reasoning,
				CommitSHA:  commit.SHA,
			})
		}
	}

	return changes
}

func groupChangesByBeadField(changes []beadChangeWithMeta) map[string][]beadChangeWithMeta {
	grouped := make(map[string][]beadChangeWithMeta)
	for _, ch := range changes {
		key := ch.BeadID + ":" + ch.Field
		grouped[key] = append(grouped[key], ch)
	}
	return grouped
}

func isConflict(changes []beadChangeWithMeta) bool {
	if len(changes) < 2 {
		return false
	}

	// Check for different values from different polecats
	uniqueValues := make(map[string]bool)
	uniquePolecats := make(map[string]bool)

	for _, ch := range changes {
		uniqueValues[ch.NewValue] = true
		uniquePolecats[ch.Polecat] = true
	}

	return len(uniqueValues) > 1 && len(uniquePolecats) > 1
}

func buildEscalationMailBody(mrID string, conflicts []SemanticConflict) string {
	var body strings.Builder

	body.WriteString(fmt.Sprintf("Semantic conflicts detected in MR: %s\n\n", mrID))

	for i, conflict := range conflicts {
		body.WriteString(fmt.Sprintf("## Conflict %d: %s.%s\n\n", i+1, conflict.BeadID, conflict.Field))

		for j, change := range conflict.Changes {
			body.WriteString(fmt.Sprintf("**Change %d** (by %s):\n", j+1, change.Polecat))
			body.WriteString(fmt.Sprintf("- Value: %s -> %s\n", change.OldValue, change.NewValue))
			if change.Confidence > 0 {
				body.WriteString(fmt.Sprintf("- Confidence: %.2f\n", change.Confidence))
			}
			if change.Reasoning != "" {
				body.WriteString(fmt.Sprintf("- Reasoning: %s\n", change.Reasoning))
			}
			if change.CommitSHA != "" {
				body.WriteString(fmt.Sprintf("- Commit: %s\n", change.CommitSHA[:8]))
			}
			body.WriteString("\n")
		}
	}

	body.WriteString("---\n")
	body.WriteString("Please review and provide a resolution.\n\n")
	body.WriteString("Reply with JSON:\n```json\n")
	body.WriteString("{\n")
	body.WriteString("  \"resolutions\": {\n")
	body.WriteString("    \"<bead_id>:<field>\": \"<resolved_value>\"\n")
	body.WriteString("  },\n")
	body.WriteString("  \"reasoning\": \"<your decision reasoning>\"\n")
	body.WriteString("}\n```\n")

	return body.String()
}

func readStdin() ([]byte, error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("no input provided (use --file or pipe to stdin)")
	}
	return os.ReadFile("/dev/stdin")
}
