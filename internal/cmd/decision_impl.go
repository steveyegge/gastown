package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/inject"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/notify"
	"github.com/steveyegge/gastown/internal/rpcclient"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
	decisionTUI "github.com/steveyegge/gastown/internal/tui/decision"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/validator"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runDecisionRequest(cmd *cobra.Command, args []string) error {
	// Validate prompt
	if decisionPrompt == "" {
		return fmt.Errorf("--prompt is required")
	}

	// Validate options (2-4 required)
	if len(decisionOptions) < 2 {
		return fmt.Errorf("at least 2 options required (use --option multiple times)")
	}
	if len(decisionOptions) > 4 {
		return fmt.Errorf("at most 4 options allowed")
	}

	// Validate urgency
	urgency := strings.ToLower(decisionUrgency)
	if !beads.IsValidUrgency(urgency) {
		return fmt.Errorf("invalid urgency '%s': must be high, medium, or low", decisionUrgency)
	}

	// Validate context is valid JSON (if provided)
	if decisionContext != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(decisionContext), &js); err != nil {
			return fmt.Errorf("--context must be valid JSON: %w\n\nExample valid JSON:\n  --context '{\"key\": \"value\"}'\n  --context '[\"item1\", \"item2\"]'\n  --context '\"simple string\"'", err)
		}
	}

	// Validate recommend index
	if decisionRecommend < 0 || decisionRecommend > len(decisionOptions) {
		return fmt.Errorf("--recommend must be between 1 and %d", len(decisionOptions))
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect agent identity
	agentID := detectSender()
	if agentID == "" {
		agentID = "unknown"
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Validate --parent if specified
	if decisionParent != "" {
		parentIssue, err := bd.Show(decisionParent)
		if err != nil {
			return fmt.Errorf("--parent validation failed: bead %q not found: %w", decisionParent, err)
		}
		if parentIssue.Type != "epic" {
			return fmt.Errorf("--parent validation failed: bead %q is type %q, but must be an epic", decisionParent, parentIssue.Type)
		}
	}

	// Enforce single-decision rule: auto-close existing pending decisions
	pendingDecisions, err := bd.ListPendingDecisionsForRequester(agentID)
	if err == nil && len(pendingDecisions) > 0 {
		for _, pending := range pendingDecisions {
			// Will be superseded by the new decision (ID assigned later)
			// For now, mark as superseded with placeholder
			if closeErr := bd.CloseDecisionAsSuperseded(pending.ID, "new-decision"); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to auto-close %s: %v\n", pending.ID, closeErr)
			} else {
				fmt.Fprintf(os.Stderr, "⚠ Auto-closed stale decision %s (superseded by new request)\n", pending.ID)
			}
		}
	}

	// Parse options
	var options []beads.DecisionOption
	for i, optStr := range decisionOptions {
		opt := beads.DecisionOption{}

		// Parse "Label: Description" format
		if colonIdx := strings.Index(optStr, ":"); colonIdx != -1 {
			opt.Label = strings.TrimSpace(optStr[:colonIdx])
			opt.Description = strings.TrimSpace(optStr[colonIdx+1:])
		} else {
			opt.Label = strings.TrimSpace(optStr)
		}

		// Mark as recommended if specified
		if decisionRecommend == i+1 {
			opt.Recommended = true
		}

		options = append(options, opt)
	}

	// Run script-based validators (unless --no-file-check skips all validation)
	if !decisionNoFileCheck {
		// Parse context as map for validators
		var contextMap map[string]interface{}
		if decisionContext != "" {
			_ = json.Unmarshal([]byte(decisionContext), &contextMap)
		}

		// Convert options for validator input
		var validatorOptions []validator.OptionInput
		for _, opt := range options {
			validatorOptions = append(validatorOptions, validator.OptionInput{
				Label:       opt.Label,
				Description: opt.Description,
				Recommended: opt.Recommended,
			})
		}

		// Build validator input
		validatorInput := validator.DecisionInput{
			Prompt:        decisionPrompt,
			Context:       contextMap,
			Options:       validatorOptions,
			PredecessorID: decisionPredecessor,
			Type:          decisionType,
		}

		// Run create validators (script-based)
		result := validator.RunCreateValidators(townRoot, validatorInput)
		if !result.Passed {
			// Show errors
			for _, e := range result.Errors {
				style.PrintError("%s", e)
			}
			// Show warnings as advice
			for _, w := range result.Warnings {
				style.PrintWarning("Advice: %s", w)
			}
			return fmt.Errorf("validation failed (use --no-file-check to skip)")
		}

		// Show any warnings even on success
		for _, w := range result.Warnings {
			style.PrintWarning("%s", w)
		}

		// Run schema bead validation (if --type specified)
		if decisionType != "" {
			schemaResult := validator.ValidateAgainstSchema(decisionType, contextMap)
			if !schemaResult.Valid && schemaResult.Blocking {
				for _, e := range schemaResult.Errors {
					style.PrintError("%s", e)
				}
				for _, w := range schemaResult.Warnings {
					style.PrintWarning("Advice: %s", w)
				}
				return fmt.Errorf("schema validation failed")
			}
			// Show warnings even on success
			for _, w := range schemaResult.Warnings {
				style.PrintWarning("%s", w)
			}
		}

		// Run referenced bead validation (unless --no-bead-check)
		if !decisionNoBeadCheck {
			if err := validateReferencedBeads(decisionPrompt, decisionContext, contextMap); err != nil {
				return err
			}
		}
	}

	// Check if predecessor suggested a successor type (blocking unless --ignore-suggested-type)
	if decisionPredecessor != "" && !decisionIgnoreSuggestedType {
		suggestedType := checkPredecessorSuggestedType(townRoot, decisionPredecessor)
		if suggestedType != "" {
			if decisionType == "" {
				style.PrintError("Predecessor suggested successor type '%s' but --type was not specified", suggestedType)
				style.PrintWarning("Use: gt decision request --type=%s ...", suggestedType)
				style.PrintWarning("Or override with: --ignore-suggested-type")
				return fmt.Errorf("successor type enforcement: predecessor suggested '%s'", suggestedType)
			} else if decisionType != suggestedType {
				style.PrintError("Predecessor suggested successor type '%s' but --type=%s was used", suggestedType, decisionType)
				style.PrintWarning("Use: gt decision request --type=%s ...", suggestedType)
				style.PrintWarning("Or override with: --ignore-suggested-type")
				return fmt.Errorf("successor type enforcement: expected '%s', got '%s'", suggestedType, decisionType)
			}
		}
	}

	// Embed type in context if specified (for Slack rendering)
	contextToStore := decisionContext
	if decisionType != "" {
		contextToStore = embedTypeInContext(decisionContext, decisionType)
	}

	// Embed session_id in context for turn enforcement
	sessionID := runtime.SessionIDFromEnv()
	if sessionID != "" {
		contextToStore = embedSessionIDInContext(contextToStore, sessionID)
	}

	// Build decision fields
	fields := &beads.DecisionFields{
		Question:      decisionPrompt,
		Context:       contextToStore,
		Options:       options,
		ChosenIndex:   0, // Pending
		RequestedBy:   agentID,
		RequestedAt:   time.Now().Format(time.RFC3339),
		Urgency:       urgency,
		PredecessorID: decisionPredecessor,
		ParentBeadID:  decisionParent,
		SessionID:     runtime.SessionIDFromEnv(), // For turn enforcement
	}

	// Add blocker if specified
	if decisionBlocks != "" {
		fields.Blockers = []string{decisionBlocks}
	}

	// Try RPC first if gtmobile is available (enables real-time event bus notifications)
	var issue *beads.Issue
	rpcUsed := false

	rpcClient := rpcclient.NewClient("http://localhost:8443")
	if rpcClient.IsAvailable(context.Background()) {
		// Convert options for RPC
		var rpcOptions []rpcclient.DecisionOption
		for _, opt := range options {
			rpcOptions = append(rpcOptions, rpcclient.DecisionOption{
				Label:       opt.Label,
				Description: opt.Description,
				Recommended: opt.Recommended,
			})
		}

		decision, rpcErr := rpcClient.CreateDecision(context.Background(), rpcclient.CreateDecisionRequest{
			Question:      decisionPrompt,
			Context:       contextToStore,
			Options:       rpcOptions,
			RequestedBy:   agentID,
			Urgency:       urgency,
			Blockers:      fields.Blockers,
			PredecessorID: fields.PredecessorID,
			ParentBead:    decisionParent,
		})
		if rpcErr == nil {
			// RPC succeeded - use the returned decision
			issue = &beads.Issue{ID: decision.ID}
			rpcUsed = true
		} else {
			// Log RPC error for debugging (hq-3p8p76)
			style.PrintWarning("RPC CreateDecision failed (falling back to direct DB): %v", rpcErr)
		}
	} else {
		style.PrintWarning("gtmobile RPC not available (falling back to direct DB)")
	}

	// Fall back to bd decision create if RPC not available or failed
	// This uses the canonical decision_points table storage (hq-946577.39)
	if !rpcUsed {
		bd := beads.New(beads.ResolveBeadsDir(townRoot))
		var err error
		issue, err = bd.CreateBdDecision(fields)
		if err != nil {
			return fmt.Errorf("creating decision: %w", err)
		}
	}

	// Note: blocker dependency is now handled by bd decision create --blocks flag

	// Send notification mail to human/overseer
	router := mail.NewRouter(townRoot)
	msg := &mail.Message{
		From:    agentID,
		To:      "overseer",
		Subject: fmt.Sprintf("[DECISION] %s", decisionPrompt),
		Body:    formatDecisionMailBody(issue.ID, fields),
		Type:    mail.TypeTask,
	}

	// Set priority based on urgency
	switch urgency {
	case beads.UrgencyHigh:
		msg.Priority = mail.PriorityHigh
	case beads.UrgencyMedium:
		msg.Priority = mail.PriorityNormal
	default:
		msg.Priority = mail.PriorityLow
	}

	if err := router.Send(msg); err != nil {
		style.PrintWarning("failed to send notification: %v", err)
	}

	// Log to activity feed
	payload := map[string]interface{}{
		"decision_id":  issue.ID,
		"question":     decisionPrompt,
		"urgency":      urgency,
		"option_count": len(options),
		"requested_by": agentID,
	}
	if decisionBlocks != "" {
		payload["blocking"] = decisionBlocks
	}
	if decisionPredecessor != "" {
		payload["predecessor_id"] = decisionPredecessor
	}
	if decisionParent != "" {
		payload["parent_id"] = decisionParent
	}
	_ = events.LogFeed(events.TypeDecisionRequested, agentID, payload)

	// Set turn marker so turn-check knows a decision was offered this turn
	// This replaces the PostToolUse hook approach which was error-prone
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		if err := createTurnMarker(sessionID); err != nil {
			// Non-fatal - just log warning
			fmt.Fprintf(os.Stderr, "Warning: failed to set turn marker: %v\n", err)
		}
	}

	// Output
	if decisionJSON {
		result := map[string]interface{}{
			"id":           issue.ID,
			"question":     decisionPrompt,
			"urgency":      urgency,
			"options":      options,
			"requested_by": agentID,
		}
		if decisionBlocks != "" {
			result["blocking"] = decisionBlocks
		}
		if decisionPredecessor != "" {
			result["predecessor_id"] = decisionPredecessor
		}
		if decisionParent != "" {
			result["parent_id"] = decisionParent
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		slug := util.GenerateDecisionSlug(issue.ID, decisionPrompt)
		fmt.Printf("📋 Decision requested: %s\n", slug)
		fmt.Printf("   Question: %s\n", decisionPrompt)
		fmt.Printf("   Options: %s\n", formatOptionsSummary(options))
		if decisionBlocks != "" {
			fmt.Printf("   Blocking: %s\n", decisionBlocks)
		}
		if decisionPredecessor != "" {
			fmt.Printf("   Predecessor: %s\n", decisionPredecessor)
		}
		if decisionParent != "" {
			fmt.Printf("   Parent: %s\n", decisionParent)
		}
		fmt.Printf("\n→ Notified human (overseer)\n")
		fmt.Printf("\nTo resolve: gt decision resolve %s --choice N --rationale \"...\"\n", issue.ID)
	}

	return nil
}

func runDecisionList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	var issues []*beads.Issue
	if decisionListAll {
		issues, err = bd.ListAllDecisions()
	} else {
		// Use ListAllPendingDecisions to include both gt and bd decisions
		issues, err = bd.ListAllPendingDecisions()
	}
	if err != nil {
		return fmt.Errorf("listing decisions: %w", err)
	}

	if decisionListJSON {
		out, _ := json.MarshalIndent(issues, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(issues) == 0 {
		fmt.Println("No pending decisions")
		return nil
	}

	statusLabel := "Pending"
	if decisionListAll {
		statusLabel = "All"
	}
	fmt.Printf("📋 %s Decisions (%d):\n\n", statusLabel, len(issues))

	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		emoji := urgencyEmoji(fields.Urgency)

		status := "PENDING"
		if beads.HasLabel(issue, "decision:resolved") {
			status = "RESOLVED"
		} else if beads.HasLabel(issue, "decision:canceled") {
			status = "CANCELED"
		}

		slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
		fmt.Printf("  %s %s [%s] %s\n", emoji, slug, status, truncateString(fields.Question, 50))
		fmt.Printf("     Requested by: %s | %s\n", fields.RequestedBy, formatRelativeTimeSimple(issue.CreatedAt))
		fmt.Printf("     Options: %s\n", formatOptionsSummary(fields.Options))
		if len(fields.Blockers) > 0 {
			fmt.Printf("     Blocking: %s\n", strings.Join(fields.Blockers, ", "))
		}
		if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
			fmt.Printf("     → Chose: %s\n", fields.Options[fields.ChosenIndex-1].Label)
		}
		fmt.Println()
	}

	return nil
}

func runDecisionShow(cmd *cobra.Command, args []string) error {
	decisionID := util.ResolveSemanticSlug(args[0])

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	if decisionJSON {
		data := map[string]interface{}{
			"id":             issue.ID,
			"question":       fields.Question,
			"context":        fields.Context,
			"options":        fields.Options,
			"chosen_index":   fields.ChosenIndex,
			"rationale":      fields.Rationale,
			"urgency":        fields.Urgency,
			"requested_by":   fields.RequestedBy,
			"requested_at":   fields.RequestedAt,
			"resolved_by":    fields.ResolvedBy,
			"resolved_at":    fields.ResolvedAt,
			"blockers":       fields.Blockers,
			"predecessor_id": fields.PredecessorID,
			"status":         issue.Status,
		}
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	emoji := urgencyEmoji(fields.Urgency)
	status := "PENDING"
	if beads.HasLabel(issue, "decision:resolved") {
		status = "RESOLVED"
	} else if beads.HasLabel(issue, "decision:canceled") {
		status = "CANCELED"
	} else if issue.Status == "closed" {
		// Bead was closed without resolution (e.g., stale cleanup)
		// Fix for gt-bug-gt_decision_show_reports_pending_closed
		status = "CLOSED"
	}

	slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
	fmt.Printf("%s Decision: %s [%s]\n\n", emoji, slug, status)
	fmt.Printf("Question: %s\n\n", fields.Question)

	if fields.Context != "" {
		fmt.Printf("Context:\n  %s\n\n", strings.ReplaceAll(fields.Context, "\n", "\n  "))
	}

	fmt.Printf("Options:\n")
	for i, opt := range fields.Options {
		num := i + 1
		marker := ""
		if opt.Recommended {
			marker = " (Recommended)"
		}
		if fields.ChosenIndex == num {
			marker += " ✓ CHOSEN"
		}
		fmt.Printf("  %d. %s%s\n", num, opt.Label, marker)
		if opt.Description != "" {
			fmt.Printf("     %s\n", opt.Description)
		}
	}
	fmt.Println()

	fmt.Printf("Requested by: %s\n", fields.RequestedBy)
	fmt.Printf("Requested at: %s\n", formatRelativeTimeSimple(fields.RequestedAt))
	fmt.Printf("Urgency: %s\n", fields.Urgency)
	if len(fields.Blockers) > 0 {
		fmt.Printf("Blocking: %s\n", strings.Join(fields.Blockers, ", "))
	}
	if fields.PredecessorID != "" {
		fmt.Printf("Predecessor: %s\n", fields.PredecessorID)
	}

	if fields.ChosenIndex > 0 {
		fmt.Println()
		fmt.Printf("Resolution:\n")
		fmt.Printf("  Chosen: %s\n", fields.Options[fields.ChosenIndex-1].Label)
		if fields.Rationale != "" {
			fmt.Printf("  Rationale: %s\n", fields.Rationale)
		}
		fmt.Printf("  Resolved by: %s\n", fields.ResolvedBy)
		fmt.Printf("  Resolved at: %s\n", formatRelativeTimeSimple(fields.ResolvedAt))
	} else {
		fmt.Printf("\nTo resolve: gt decision resolve %s --choice N --rationale \"...\"\n", issue.ID)
	}

	return nil
}

func runDecisionResolve(cmd *cobra.Command, args []string) error {
	decisionID := util.ResolveSemanticSlug(args[0])

	if decisionChoice < 1 {
		return fmt.Errorf("--choice is required and must be >= 1")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect who is resolving
	resolvedBy := detectSender()
	if resolvedBy == "" {
		resolvedBy = "human"
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get the decision first to validate and get info for notifications
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	// Validate choice
	if decisionChoice > len(fields.Options) {
		return fmt.Errorf("invalid choice %d: only %d options available", decisionChoice, len(fields.Options))
	}

	chosenOption := fields.Options[decisionChoice-1]

	// Use option description as fallback rationale if none provided
	effectiveRationale := decisionRationale
	if effectiveRationale == "" && chosenOption.Description != "" {
		effectiveRationale = chosenOption.Description
	}

	// Try RPC first if gtmobile is available (enables real-time event bus notifications)
	rpcUsed := false
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	if rpcClient.IsAvailable(context.Background()) {
		_, rpcErr := rpcClient.ResolveDecision(context.Background(), decisionID, decisionChoice, effectiveRationale, resolvedBy)
		if rpcErr == nil {
			rpcUsed = true
		} else {
			// Log RPC error for debugging
			style.PrintWarning("RPC Resolve failed (falling back to direct BD): %v", rpcErr)
		}
	}

	// Fall back to direct BD if RPC not available or failed
	if !rpcUsed {
		if err := bd.ResolveDecision(decisionID, decisionChoice, effectiveRationale, resolvedBy); err != nil {
			return fmt.Errorf("resolving decision: %w", err)
		}
	}

	// Notify requestor: mail + nudge + unblock + activity log
	notify.DecisionResolved(townRoot, decisionID, *fields, chosenOption.Label, effectiveRationale, resolvedBy)

	// Output
	if decisionJSON {
		result := map[string]interface{}{
			"id":           decisionID,
			"chosen_index": decisionChoice,
			"chosen_label": chosenOption.Label,
			"rationale":    effectiveRationale,
			"resolved_by":  resolvedBy,
			"unblocked":    fields.Blockers,
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("✓ Resolved %s: %s\n", decisionID, chosenOption.Label)
		if effectiveRationale != "" {
			fmt.Printf("  Rationale: %s\n", effectiveRationale)
		}
		if len(fields.Blockers) > 0 {
			fmt.Printf("\n→ Unblocked: %s\n", strings.Join(fields.Blockers, ", "))
		}
		if fields.RequestedBy != "" && fields.RequestedBy != "unknown" {
			fmt.Printf("→ Notified: %s\n", fields.RequestedBy)
		}
	}

	return nil
}

func runDecisionDashboard(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get pending decisions grouped by urgency
	pendingHigh, _ := bd.ListDecisionsByUrgency(beads.UrgencyHigh)
	pendingMedium, _ := bd.ListDecisionsByUrgency(beads.UrgencyMedium)
	pendingLow, _ := bd.ListDecisionsByUrgency(beads.UrgencyLow)

	// Get recently resolved (last 7 days)
	recentlyResolved, _ := bd.ListRecentlyResolvedDecisions(7 * 24 * time.Hour)

	// Get stale decisions (older than 24 hours)
	staleDecisions, _ := bd.ListStaleDecisions(24 * time.Hour)

	totalPending := len(pendingHigh) + len(pendingMedium) + len(pendingLow)

	if decisionDashboardJSON {
		result := map[string]interface{}{
			"pending": map[string]interface{}{
				"high":   formatDecisionsList(pendingHigh),
				"medium": formatDecisionsList(pendingMedium),
				"low":    formatDecisionsList(pendingLow),
				"total":  totalPending,
			},
			"recently_resolved": formatDecisionsList(recentlyResolved),
			"stale":             formatDecisionsList(staleDecisions),
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Println("📋 Decision Dashboard")
	fmt.Println()

	// Pending section
	fmt.Printf("Pending (%d)\n", totalPending)
	if totalPending == 0 {
		fmt.Println("  (none)")
	} else {
		// High urgency first
		for _, issue := range pendingHigh {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
			fmt.Printf("  🔴 [HIGH] %s: %s (%s)\n", slug, truncateString(fields.Question, 40), age)
		}
		// Medium urgency
		for _, issue := range pendingMedium {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
			fmt.Printf("  🟡 [MEDIUM] %s: %s (%s)\n", slug, truncateString(fields.Question, 40), age)
		}
		// Low urgency
		for _, issue := range pendingLow {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
			fmt.Printf("  🟢 [LOW] %s: %s (%s)\n", slug, truncateString(fields.Question, 40), age)
		}
	}
	fmt.Println()

	// Recently resolved section
	fmt.Printf("Recently Resolved (%d)\n", len(recentlyResolved))
	if len(recentlyResolved) == 0 {
		fmt.Println("  (none in last 7 days)")
	} else {
		for i, issue := range recentlyResolved {
			if i >= 5 {
				fmt.Printf("  ... and %d more\n", len(recentlyResolved)-5)
				break
			}
			fields := beads.ParseDecisionFields(issue.Description)
			chosen := "?"
			if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
				chosen = fields.Options[fields.ChosenIndex-1].Label
			}
			age := formatDecisionAge(issue.ClosedAt)
			slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
			fmt.Printf("  ✓ %s: %s → \"%s\" (%s)\n", slug, truncateString(fields.Question, 30), chosen, age)
		}
	}
	fmt.Println()

	// Stale section
	if len(staleDecisions) > 0 {
		fmt.Printf("⚠️  Stale (unresolved > 24h): %d\n", len(staleDecisions))
		for _, issue := range staleDecisions {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
			fmt.Printf("  ⚠️  %s: %s (%s old)\n", slug, truncateString(fields.Question, 40), age)
		}
		fmt.Println()
	}

	fmt.Println("Run 'gt decision list' for details")

	return nil
}

func formatDecisionsList(issues []*beads.Issue) []map[string]interface{} {
	var result []map[string]interface{}
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		item := map[string]interface{}{
			"id":           issue.ID,
			"question":     fields.Question,
			"urgency":      fields.Urgency,
			"requested_by": fields.RequestedBy,
			"created_at":   issue.CreatedAt,
		}
		if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
			item["chosen"] = fields.Options[fields.ChosenIndex-1].Label
		}
		result = append(result, item)
	}
	return result
}

func formatDecisionAge(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Helper functions

func formatDecisionMailBody(beadID string, fields *beads.DecisionFields) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Decision ID: %s", beadID))
	lines = append(lines, fmt.Sprintf("Urgency: %s", fields.Urgency))
	lines = append(lines, fmt.Sprintf("From: %s", fields.RequestedBy))
	lines = append(lines, "")
	lines = append(lines, "Question:")
	lines = append(lines, fields.Question)

	if fields.Context != "" {
		lines = append(lines, "")
		lines = append(lines, "Context:")
		lines = append(lines, fields.Context)
	}

	lines = append(lines, "")
	lines = append(lines, "Options:")
	for i, opt := range fields.Options {
		marker := ""
		if opt.Recommended {
			marker = " (Recommended)"
		}
		lines = append(lines, fmt.Sprintf("  %d. %s%s", i+1, opt.Label, marker))
		if opt.Description != "" {
			lines = append(lines, fmt.Sprintf("     %s", opt.Description))
		}
	}

	if len(fields.Blockers) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Blocking: %s", strings.Join(fields.Blockers, ", ")))
	}

	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, fmt.Sprintf("To resolve: gt decision resolve %s --choice N --rationale \"...\"", beadID))
	return strings.Join(lines, "\n")
}

func formatResolutionMailBody(beadID, question, chosen, rationale, resolvedBy string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Decision ID: %s", beadID))
	lines = append(lines, fmt.Sprintf("Question: %s", question))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Chosen: %s", chosen))
	if rationale != "" {
		lines = append(lines, fmt.Sprintf("Rationale: %s", rationale))
	}
	lines = append(lines, fmt.Sprintf("Resolved by: %s", resolvedBy))
	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, "This decision has been resolved. Any blocked work should now be unblocked.")
	return strings.Join(lines, "\n")
}

func formatOptionsSummary(options []beads.DecisionOption) string {
	var labels []string
	for _, opt := range options {
		label := opt.Label
		if opt.Recommended {
			label += "*"
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func urgencyEmoji(urgency string) string {
	switch urgency {
	case beads.UrgencyHigh:
		return "🔴"
	case beads.UrgencyMedium:
		return "🟡"
	case beads.UrgencyLow:
		return "🟢"
	default:
		return "📋"
	}
}

func runDecisionAwait(cmd *cobra.Command, args []string) error {
	decisionID := util.ResolveSemanticSlug(args[0])

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Parse timeout if specified
	var timeout time.Duration
	if decisionAwaitTimeout != "" {
		timeout, err = time.ParseDuration(decisionAwaitTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Check if already resolved
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	startTime := time.Now()
	pollInterval := 5 * time.Second

	for {
		// Check if resolved
		if fields.ChosenIndex > 0 {
			// Decision is resolved
			if decisionJSON {
				result := map[string]interface{}{
					"id":           decisionID,
					"resolved":     true,
					"chosen_index": fields.ChosenIndex,
					"chosen_label": fields.Options[fields.ChosenIndex-1].Label,
					"rationale":    fields.Rationale,
					"resolved_by":  fields.ResolvedBy,
					"resolved_at":  fields.ResolvedAt,
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			} else {
				fmt.Printf("✓ Decision %s resolved: %s\n", decisionID, fields.Options[fields.ChosenIndex-1].Label)
				if fields.Rationale != "" {
					fmt.Printf("  Rationale: %s\n", fields.Rationale)
				}
			}
			return nil
		}

		// Check timeout
		if timeout > 0 && time.Since(startTime) > timeout {
			if decisionJSON {
				result := map[string]interface{}{
					"id":       decisionID,
					"resolved": false,
					"error":    "timeout waiting for decision",
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			}
			return fmt.Errorf("timeout waiting for decision %s to be resolved", decisionID)
		}

		// Wait and poll again
		time.Sleep(pollInterval)

		// Refresh decision state
		issue, fields, err = bd.GetDecisionBead(decisionID)
		if err != nil {
			return fmt.Errorf("getting decision: %w", err)
		}
		if issue == nil {
			return fmt.Errorf("decision not found: %s", decisionID)
		}
	}
}

func runDecisionRemind(cmd *cobra.Command, args []string) error {
	// Detect if there's work in the session that warrants a decision
	hasWork := false
	var workIndicators []string

	// Check 1: Git status - uncommitted changes
	gitChanges := checkGitChanges()
	if gitChanges != "" {
		hasWork = true
		workIndicators = append(workIndicators, gitChanges)
	}

	// Check 2: Hooked work
	hookedWork := checkHookedWork()
	if hookedWork != "" {
		hasWork = true
		workIndicators = append(workIndicators, hookedWork)
	}

	// Check 3: In-progress beads
	inProgressBeads := checkInProgressBeads()
	if inProgressBeads != "" {
		hasWork = true
		workIndicators = append(workIndicators, inProgressBeads)
	}

	if !hasWork {
		if decisionRemindInject || decisionRemindNudge {
			// Silent exit - no reminder needed
			return nil
		}
		fmt.Println("No session work detected - no decision reminder needed")
		return nil
	}

	// Format the reminder
	reminderText := formatDecisionReminder(workIndicators)

	if decisionRemindNudge {
		// Send reminder as nudge to current agent's session
		agent := detectSender()
		if agent == "" {
			agent = "gastown/crew/decision_point" // fallback
		}
		nudgeMsg := "DECISION REMINDER: Session work detected. Consider offering the user a decision point about next steps before ending this session."
		nudgeCmd := execCommand("gt", "nudge", agent, nudgeMsg)
		if err := nudgeCmd.Run(); err != nil {
			// Don't fail the hook, just log
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to send decision nudge: %v\n", err)
		}
		return nil
	}

	if decisionRemindInject {
		// Output as system-reminder for Claude Code hooks
		fmt.Printf("<system-reminder>\n%s\n</system-reminder>\n", reminderText)
		return nil
	}

	// Human-readable output
	fmt.Println(reminderText)
	return nil
}

func checkGitChanges() string {
	// Check for uncommitted changes using git status
	out, err := runGitCommand("status", "--porcelain")
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return ""
	}
	return fmt.Sprintf("uncommitted changes (%d files)", len(lines))
}

func checkHookedWork() string {
	// Check for hooked work via gt hook
	out, err := runCommand("gt", "hook", "--json")
	if err != nil {
		return ""
	}
	// Parse JSON to check if there's a hooked bead
	var hookData map[string]interface{}
	if err := json.Unmarshal([]byte(out), &hookData); err != nil {
		return ""
	}
	if beadID, ok := hookData["bead_id"].(string); ok && beadID != "" {
		return fmt.Sprintf("hooked work: %s", beadID)
	}
	return ""
}

func checkInProgressBeads() string {
	// Check for in-progress beads
	out, err := runCommand("bd", "list", "--status=in_progress", "--json")
	if err != nil {
		return ""
	}
	var beadsList []interface{}
	if err := json.Unmarshal([]byte(out), &beadsList); err != nil {
		return ""
	}
	if len(beadsList) > 0 {
		return fmt.Sprintf("in-progress beads (%d)", len(beadsList))
	}
	return ""
}

func runGitCommand(args ...string) (string, error) {
	cmd := execCommand("git", args...)
	out, err := cmd.Output()
	return string(out), err
}

func runCommand(name string, args ...string) (string, error) {
	cmd := execCommand(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// execCommand is a wrapper for os/exec.Command
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) //nolint:gosec // trusted internal commands
}

func formatDecisionReminder(workIndicators []string) string {
	var sb strings.Builder
	sb.WriteString("DECISION OFFERING REMINDER\n")
	sb.WriteString("\n")
	sb.WriteString("Session work detected:\n")
	for _, indicator := range workIndicators {
		sb.WriteString(fmt.Sprintf("  - %s\n", indicator))
	}
	sb.WriteString("\n")
	sb.WriteString("Before ending this session, consider offering a decision point:\n")
	sb.WriteString("1. What was accomplished in this session?\n")
	sb.WriteString("2. What are the next steps or options?\n")
	sb.WriteString("3. Are there architectural choices or scope decisions needed?\n")
	sb.WriteString("\n")
	sb.WriteString("Use 'gt decision request' to create a decision, or proceed with handoff if\n")
	sb.WriteString("the work is self-contained and no human input is needed.\n")
	return sb.String()
}

func runDecisionWatch(cmd *cobra.Command, args []string) error {
	// Handle RPC mode - simple streaming watcher for testing the RPC layer
	if decisionWatchRPC {
		return runDecisionWatchRPC()
	}

	// Verify we're in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Try to infer current rig
	currentRig, _ := inferRigFromCwd(townRoot)
	if currentRig == "" {
		currentRig = "gastown" // fallback
	}

	// Create the TUI model
	m := decisionTUI.New()

	// Set workspace info for crew creation
	m.SetWorkspace(townRoot, currentRig)

	// Apply flags
	if decisionWatchUrgentOnly {
		m.SetFilter("high")
	}
	if decisionWatchNotify {
		m.SetNotify(true)
	}

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running decision watch: %w", err)
	}

	return nil
}

// runDecisionWatchRPC runs the decision TUI backed by RPC calls.
// This provides the same interactive experience as the TUI mode but
// communicates with a remote RPC server instead of using local beads.
func runDecisionWatchRPC() error {
	// Check RPC server availability first
	client := rpcclient.NewClient(decisionWatchRPCAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if !client.IsAvailable(ctx) {
		cancel()
		return fmt.Errorf("RPC server not available at %s", decisionWatchRPCAddr)
	}
	cancel()

	// Create the TUI model configured for RPC mode
	m := decisionTUI.New()
	m.SetRPCClient(client)

	// Apply flags (same as non-RPC mode)
	if decisionWatchUrgentOnly {
		m.SetFilter("high")
	}
	if decisionWatchNotify {
		m.SetNotify(true)
	}

	// Note: workspace/rig info not available in pure RPC mode,
	// so crew creation will be disabled

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running decision watch: %w", err)
	}

	return nil
}

// --- Turn enforcement functions ---

// turnMarkerPath returns the path to the turn marker file for a session.
func turnMarkerPath(sessionID string) string {
	return fmt.Sprintf("/tmp/.decision-offered-%s", sessionID)
}

// turnMarkerExists checks if the turn marker exists.
func turnMarkerExists(sessionID string) bool {
	_, err := os.Stat(turnMarkerPath(sessionID))
	return err == nil
}

// createTurnMarker creates the turn marker file.
func createTurnMarker(sessionID string) error {
	f, err := os.Create(turnMarkerPath(sessionID))
	if err != nil {
		return err
	}
	return f.Close()
}

// clearTurnMarker removes the turn marker file.
func clearTurnMarker(sessionID string) {
	_ = os.Remove(turnMarkerPath(sessionID))
}

// isDecisionCommand checks if a command creates a decision.
func isDecisionCommand(command string) bool {
	return strings.Contains(command, "gt decision request") ||
		strings.Contains(command, "bd decision create")
}

// TurnBlockResult is the JSON response for blocking a turn.
type TurnBlockResult struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// checkTurnMarker checks if a decision was offered this turn.
// Returns nil if allowed, or a TurnBlockResult if blocked.
// If soft is true, never blocks (just returns nil).
// NOTE: This does NOT clear the marker - that's done by turn-clear at the
// start of the next turn. This allows Stop hook to fire multiple times
// without incorrectly blocking on subsequent checks.
func checkTurnMarker(sessionID string, soft bool) *TurnBlockResult {
	if turnMarkerExists(sessionID) {
		// Decision was offered - allow (don't clear; turn-clear handles that)
		return nil
	}

	// No decision offered
	if soft {
		// Soft mode - don't block
		return nil
	}

	// Strict mode - block
	return &TurnBlockResult{
		Decision: "block",
		Reason: `You must offer a formal decision point using 'gt decision request' before ending this turn. This ensures humans stay informed about progress and can provide guidance.

When the decision is created, it will be assigned a semantic slug (e.g., gt-dec-cache_strategyzfyl8) that makes it easy to identify in Slack and logs. Use clear, descriptive prompts so the generated slug is meaningful.`,
	}
}

// turnHookInput represents the JSON input from Claude Code hooks for turn enforcement.
type turnHookInput struct {
	SessionID string `json:"session_id"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// readTurnHookInput reads and parses hook JSON from stdin.
// Returns an error if stdin has no data available (not called from a hook).
func readTurnHookInput() (*turnHookInput, error) {
	// Check stdin mode to determine if we're called from a hook
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat stdin: %w", err)
	}

	// If stdin is a terminal (CharDevice), we're not being called from a hook
	if fileInfo.Mode()&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("stdin is a terminal, not a hook")
	}

	// If stdin is a regular file or /dev/null (size 0), check if it has data
	// For pipes (ModeNamedPipe), we need to read - but hooks always provide data
	if fileInfo.Mode()&os.ModeNamedPipe == 0 {
		// Not a pipe - check size
		if fileInfo.Size() == 0 {
			return nil, fmt.Errorf("stdin is empty, not a hook")
		}
	}

	// For pipes, use a timeout to avoid blocking indefinitely
	// Claude Code hooks always provide data immediately
	done := make(chan struct{})
	var data []byte
	var readErr error

	go func() {
		data, readErr = io.ReadAll(os.Stdin)
		close(done)
	}()

	select {
	case <-done:
		// Read completed
	case <-time.After(100 * time.Millisecond):
		// Timeout - stdin had no data, not a hook call
		return nil, fmt.Errorf("stdin read timeout, not a hook")
	}

	if readErr != nil {
		return nil, readErr
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("stdin is empty, not a hook")
	}

	var input turnHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

func runDecisionTurnClear(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		// Hooks should never fail - just exit cleanly
		return nil
	}

	if input.SessionID != "" {
		clearTurnMarker(input.SessionID)
	}
	return nil
}

func runDecisionTurnMark(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		// Hooks should never fail
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	// Check if this is a decision command
	if isDecisionCommand(input.ToolInput.Command) {
		_ = createTurnMarker(input.SessionID)
	}

	return nil
}

func runDecisionTurnCheck(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		if decisionTurnCheckVerbose {
			fmt.Fprintf(os.Stderr, "[turn-check] Failed to read hook input: %v\n", err)
		}
		// Hooks should never fail
		return nil
	}

	if decisionTurnCheckVerbose {
		fmt.Fprintf(os.Stderr, "[turn-check] Session ID: %s\n", input.SessionID)
		fmt.Fprintf(os.Stderr, "[turn-check] Last command: %s\n", input.ToolInput.Command)
	}

	if input.SessionID == "" {
		if decisionTurnCheckVerbose {
			fmt.Fprintf(os.Stderr, "[turn-check] No session ID, skipping check\n")
		}
		return nil
	}

	if decisionTurnCheckVerbose {
		fmt.Fprintf(os.Stderr, "[turn-check] Soft mode: %v\n", decisionTurnCheckSoft)
	}

	// Check turn marker file first (fast, local) - created by gt decision request
	if turnMarkerExists(input.SessionID) {
		if decisionTurnCheckVerbose {
			fmt.Fprintf(os.Stderr, "[turn-check] OK: Turn marker file exists\n")
		}
		return nil
	}

	// Soft mode doesn't block
	if decisionTurnCheckSoft {
		if decisionTurnCheckVerbose {
			fmt.Fprintf(os.Stderr, "[turn-check] OK: Soft mode, not blocking\n")
		}
		return nil
	}

	// No decision offered this turn - block
	if decisionTurnCheckVerbose {
		fmt.Fprintf(os.Stderr, "[turn-check] BLOCKING: No decision offered this turn\n")
	}

	result := &TurnBlockResult{
		Decision: "block",
		Reason: `You must offer a formal decision point using 'gt decision request' before ending this turn. This ensures humans stay informed about progress and can provide guidance.

Example:
  gt decision request \
    --prompt "What should we do next?" \
    --option "Continue: Keep working on current task" \
    --option "Pause: Wait for further guidance"

When the decision is created, it will be assigned a semantic slug (e.g., gt-dec-cache_strategyzfyl8) that makes it easy to identify in Slack and logs. Use clear, descriptive prompts so the generated slug is meaningful.`,
	}
	out, _ := json.Marshal(result)
	fmt.Println(string(out))
	return NewSilentExit(1)
}

// checkAgentHasPendingDecisions checks if the current agent has any pending decisions.
// Returns true if there are pending decisions from this agent, false otherwise.
func checkAgentHasPendingDecisions() bool {
	// Get agent identity
	agentID := detectSender()
	if agentID == "" || agentID == "unknown" {
		return false
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return false
	}

	// Get pending decisions
	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	issues, err := bd.ListAllPendingDecisions()
	if err != nil {
		return false
	}

	// Check if any pending decisions are from this agent
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		if fields.RequestedBy == agentID {
			return true
		}
	}

	return false
}

func runDecisionCancel(cmd *cobra.Command, args []string) error {
	decisionID := util.ResolveSemanticSlug(args[0])

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a workspace: %w", err)
	}

	// Connect to beads
	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get the decision to verify it exists
	issue, _, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("failed to get decision %s: %w", decisionID, err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	// Verify it's a pending decision (has decision:pending label)
	isPending := false
	for _, label := range issue.Labels {
		if label == "decision:pending" {
			isPending = true
			break
		}
	}

	if !isPending {
		return fmt.Errorf("%s is not a pending decision (may already be resolved)", decisionID)
	}

	// Build new labels: remove decision:pending, add decision:canceled
	newLabels := []string{}
	for _, label := range issue.Labels {
		if label != "decision:pending" {
			newLabels = append(newLabels, label)
		}
	}
	newLabels = append(newLabels, "decision:canceled")

	// Close the decision with cancellation reason
	reason := decisionCancelReason
	if reason == "" {
		reason = "Canceled"
	}

	if err := bd.CloseWithReason(reason, decisionID); err != nil {
		return fmt.Errorf("failed to cancel decision: %w", err)
	}

	// Update labels
	if err := bd.Update(decisionID, beads.UpdateOptions{SetLabels: newLabels}); err != nil {
		// Non-fatal, decision is already closed
		fmt.Fprintf(os.Stderr, "Warning: failed to update labels: %v\n", err)
	}

	fmt.Printf("✓ Canceled %s: %s\n", decisionID, reason)
	return nil
}

func runDecisionAutoClose(cmd *cobra.Command, args []string) error {
	// Parse threshold duration
	threshold, err := time.ParseDuration(decisionAutoCloseThreshold)
	if err != nil {
		if decisionAutoCloseInject {
			return nil // Hooks should never fail
		}
		return fmt.Errorf("invalid threshold '%s': %w", decisionAutoCloseThreshold, err)
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		if decisionAutoCloseInject {
			return nil // Hooks should never fail
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect agent identity for filtering
	agentID := detectSender()

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get stale decisions for this agent
	staleDecisions, err := bd.ListStaleDecisions(threshold)
	if err != nil {
		if decisionAutoCloseInject {
			return nil // Hooks should never fail
		}
		return fmt.Errorf("listing stale decisions: %w", err)
	}

	// Filter to decisions requested by this agent
	var toClose []*beads.Issue
	for _, issue := range staleDecisions {
		fields := beads.ParseDecisionFields(issue.Description)
		if fields.RequestedBy == agentID {
			toClose = append(toClose, issue)
		}
	}

	if len(toClose) == 0 {
		if !decisionAutoCloseInject && !decisionAutoCloseDryRun {
			fmt.Println("No stale decisions to close")
		}
		return nil
	}

	// Dry run: just show what would be closed
	if decisionAutoCloseDryRun {
		fmt.Printf("Would close %d stale decision(s):\n", len(toClose))
		for _, issue := range toClose {
			fmt.Printf("  - %s: %s\n", issue.ID, issue.Title)
		}
		return nil
	}

	// Close each stale decision
	var closed []string
	for _, issue := range toClose {
		reason := fmt.Sprintf("Stale: no response after %s", threshold)
		if closeErr := bd.CloseWithReason(reason, issue.ID); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close %s: %v\n", issue.ID, closeErr)
			continue
		}

		// Update labels
		newLabels := []string{}
		for _, label := range issue.Labels {
			if label != "decision:pending" {
				newLabels = append(newLabels, label)
			}
		}
		newLabels = append(newLabels, "decision:stale")
		_ = bd.Update(issue.ID, beads.UpdateOptions{SetLabels: newLabels})

		closed = append(closed, issue.ID)
	}

	// Output
	if decisionAutoCloseInject {
		if len(closed) > 0 {
			fmt.Printf("<system-reminder>\n⚠ Auto-closed %d stale decision(s): %s\n</system-reminder>\n",
				len(closed), strings.Join(closed, ", "))
		}
	} else {
		fmt.Printf("✓ Closed %d stale decision(s): %s\n", len(closed), strings.Join(closed, ", "))
	}

	return nil
}

func runDecisionCheck(cmd *cobra.Command, args []string) error {
	// Determine identity (priority: --identity flag, auto-detect)
	identity := ""
	if decisionCheckIdentity != "" {
		identity = decisionCheckIdentity
	} else {
		identity = detectSender()
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		if decisionCheckInject {
			// Inject mode: always exit 0, silent on error
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get pending decisions
	issues, err := bd.ListDecisions()
	if err != nil {
		if decisionCheckInject {
			return nil
		}
		return fmt.Errorf("listing decisions: %w", err)
	}

	// Filter to decisions requested by this identity (or all if identity is overseer/human)
	var relevant []*beads.Issue
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		// Include if:
		// 1. Requested by this identity
		// 2. Identity is "overseer" or "human" (sees all)
		// 3. Has this identity in blockers (waiting on decision to unblock)
		if fields.RequestedBy == identity ||
			identity == "overseer" ||
			identity == "human" ||
			containsString(fields.Blockers, identity) {
			relevant = append(relevant, issue)
		}
	}

	// Check for recently-resolved decisions (last 1 hour) relevant to this identity
	resolved, _ := bd.ListRecentlyResolvedDecisions(1 * time.Hour)
	var relevantResolved []*beads.Issue
	for _, issue := range resolved {
		fields := beads.ParseDecisionFields(issue.Description)
		if fields.RequestedBy == identity ||
			identity == "overseer" ||
			identity == "human" {
			relevantResolved = append(relevantResolved, issue)
		}
	}

	// JSON output
	if decisionCheckJSON {
		result := map[string]interface{}{
			"identity":          identity,
			"pending":           len(relevant),
			"recently_resolved": len(relevantResolved),
			"has_new":           len(relevant) > 0 || len(relevantResolved) > 0,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Inject mode: queue system-reminder if decisions exist
	if decisionCheckInject {
		hasContent := len(relevant) > 0 || len(relevantResolved) > 0
		if hasContent {
			var buf bytes.Buffer
			buf.WriteString("<system-reminder>\n")

			if len(relevant) > 0 {
				buf.WriteString(fmt.Sprintf("You have %d pending decision(s) awaiting resolution.\n\n", len(relevant)))
				for _, issue := range relevant {
					fields := beads.ParseDecisionFields(issue.Description)
					emoji := urgencyEmoji(fields.Urgency)
					slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
					buf.WriteString(fmt.Sprintf("- %s %s [%s]: %s\n", emoji, slug, strings.ToUpper(fields.Urgency), truncateString(fields.Question, 50)))
				}
				buf.WriteString("\n")
				buf.WriteString("Run 'gt decision list' to see details, or 'gt decision resolve <id> --choice N' to resolve.\n")
			}

			if len(relevantResolved) > 0 {
				if len(relevant) > 0 {
					buf.WriteString("\n")
				}
				buf.WriteString(fmt.Sprintf("%d of your decision(s) were recently resolved:\n\n", len(relevantResolved)))
				for _, issue := range relevantResolved {
					fields := beads.ParseDecisionFields(issue.Description)
					chosenLabel := "(unknown)"
					if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
						chosenLabel = fields.Options[fields.ChosenIndex-1].Label
					}
					slug := util.GenerateDecisionSlug(issue.ID, fields.Question)
					buf.WriteString(fmt.Sprintf("- %s: Chose \"%s\"", slug, chosenLabel))
					if fields.Rationale != "" {
						buf.WriteString(fmt.Sprintf(" - %s", truncateString(fields.Rationale, 60)))
					}
					buf.WriteString("\n")
				}
				buf.WriteString("\nYour blocked work should now be unblocked. Run 'gt decision show <id>' for details.\n")
			}

			buf.WriteString("</system-reminder>\n")

			sessionID := runtime.SessionIDFromEnv()
			if sessionID != "" {
				queue := inject.NewQueue(townRoot, sessionID)
				if err := queue.Enqueue(inject.TypeDecision, buf.String()); err != nil {
					fmt.Print(buf.String())
				}
			} else {
				fmt.Print(buf.String())
			}
		}
		return nil
	}

	// Normal mode
	if len(relevant) > 0 {
		fmt.Printf("%s %d pending decision(s)\n", style.Bold.Render("📋"), len(relevant))
	}
	if len(relevantResolved) > 0 {
		fmt.Printf("%s %d recently resolved decision(s)\n", style.Bold.Render("✓"), len(relevantResolved))
	}
	if len(relevant) > 0 || len(relevantResolved) > 0 {
		return NewSilentExit(0)
	}
	fmt.Println("No pending decisions")
	return NewSilentExit(1)
}

// containsString checks if a slice contains a string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// --- Decision Chain Visualization ---

// chainNode represents a node in the decision chain tree.
type chainNode struct {
	ID           string       `json:"id"`
	Question     string       `json:"question"`
	ChosenIndex  int          `json:"chosen_index"`
	ChosenLabel  string       `json:"chosen_label,omitempty"`
	Urgency      string       `json:"urgency"`
	RequestedBy  string       `json:"requested_by"`
	RequestedAt  string       `json:"requested_at"`
	ResolvedAt   string       `json:"resolved_at,omitempty"`
	Predecessor  string       `json:"predecessor_id,omitempty"`
	Children     []*chainNode `json:"children,omitempty"`
	IsTarget     bool         `json:"is_target,omitempty"` // Marks the requested decision
}

func runDecisionChain(cmd *cobra.Command, args []string) error {
	decisionID := util.ResolveSemanticSlug(args[0])

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	if decisionChainDescendants {
		return showDecisionDescendants(bd, decisionID)
	}

	return showDecisionAncestry(bd, decisionID)
}

// showDecisionAncestry shows the chain from root to the specified decision.
func showDecisionAncestry(bd *beads.Beads, decisionID string) error {
	// Build chain by following predecessor links
	var chain []*chainNode
	currentID := decisionID

	for currentID != "" {
		issue, fields, err := bd.GetDecisionBead(currentID)
		if err != nil {
			return fmt.Errorf("failed to load decision %s: %w", currentID, err)
		}
		if issue == nil || fields == nil {
			return fmt.Errorf("decision not found: %s", currentID)
		}

		node := &chainNode{
			ID:          issue.ID,
			Question:    fields.Question,
			ChosenIndex: fields.ChosenIndex,
			Urgency:     fields.Urgency,
			RequestedBy: fields.RequestedBy,
			RequestedAt: fields.RequestedAt,
			ResolvedAt:  fields.ResolvedAt,
			Predecessor: fields.PredecessorID,
			IsTarget:    issue.ID == decisionID,
		}

		if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
			node.ChosenLabel = fields.Options[fields.ChosenIndex-1].Label
		}

		chain = append([]*chainNode{node}, chain...) // Prepend to build root->leaf order
		currentID = fields.PredecessorID
	}

	if decisionChainJSON {
		out, _ := json.MarshalIndent(chain, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// Pretty print the chain
	fmt.Printf("📋 Decision Chain (%d decisions)\n\n", len(chain))

	for i, node := range chain {
		indent := strings.Repeat("  ", i)
		connector := "└─"
		if i == 0 {
			connector = "┌─"
		} else if i < len(chain)-1 {
			connector = "├─"
		}

		status := "PENDING"
		if node.ChosenIndex > 0 {
			status = "RESOLVED"
		}

		marker := ""
		if node.IsTarget {
			marker = " ← (target)"
		}

		emoji := urgencyEmoji(node.Urgency)
		slug := util.GenerateDecisionSlug(node.ID, node.Question)
		fmt.Printf("%s%s %s %s [%s]%s\n", indent, connector, emoji, slug, status, marker)
		fmt.Printf("%s   %s\n", indent, truncateString(node.Question, 60))

		if node.ChosenLabel != "" {
			fmt.Printf("%s   → Chose: %s\n", indent, node.ChosenLabel)
		}

		if i < len(chain)-1 {
			fmt.Printf("%s │\n", indent)
		}
	}

	return nil
}

// showDecisionDescendants shows decisions that follow from the specified decision.
func showDecisionDescendants(bd *beads.Beads, decisionID string) error {
	// Get all decisions and build a map of predecessor -> children
	allDecisions, err := bd.ListAllDecisions()
	if err != nil {
		return fmt.Errorf("failed to list decisions: %w", err)
	}

	// Build descendant map
	children := make(map[string][]string)
	for _, issue := range allDecisions {
		_, fields, err := bd.GetDecisionBead(issue.ID)
		if err != nil || fields == nil {
			continue
		}
		if fields.PredecessorID != "" {
			children[fields.PredecessorID] = append(children[fields.PredecessorID], issue.ID)
		}
	}

	// Build tree recursively
	root, err := buildChainTree(bd, decisionID, children)
	if err != nil {
		return err
	}

	if decisionChainJSON {
		out, _ := json.MarshalIndent(root, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// Pretty print tree
	fmt.Printf("📋 Decision Descendants\n\n")
	printChainTree(root, "", true)

	return nil
}

// buildChainTree builds a tree of decisions from the given root.
func buildChainTree(bd *beads.Beads, rootID string, children map[string][]string) (*chainNode, error) {
	issue, fields, err := bd.GetDecisionBead(rootID)
	if err != nil {
		return nil, fmt.Errorf("failed to load decision %s: %w", rootID, err)
	}
	if issue == nil || fields == nil {
		return nil, fmt.Errorf("decision not found: %s", rootID)
	}

	node := &chainNode{
		ID:          issue.ID,
		Question:    fields.Question,
		ChosenIndex: fields.ChosenIndex,
		Urgency:     fields.Urgency,
		RequestedBy: fields.RequestedBy,
		RequestedAt: fields.RequestedAt,
		ResolvedAt:  fields.ResolvedAt,
		Predecessor: fields.PredecessorID,
	}

	if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
		node.ChosenLabel = fields.Options[fields.ChosenIndex-1].Label
	}

	// Add children
	for _, childID := range children[rootID] {
		child, err := buildChainTree(bd, childID, children)
		if err != nil {
			continue // Skip children that can't be loaded
		}
		node.Children = append(node.Children, child)
	}

	return node, nil
}

// printChainTree prints a decision tree with ASCII art.
func printChainTree(node *chainNode, prefix string, isLast bool) {
	connector := "├─"
	if isLast {
		connector = "└─"
	}
	if prefix == "" {
		connector = "┌─" // Root node
	}

	status := "PENDING"
	if node.ChosenIndex > 0 {
		status = "RESOLVED"
	}

	emoji := urgencyEmoji(node.Urgency)
	slug := util.GenerateDecisionSlug(node.ID, node.Question)
	fmt.Printf("%s%s %s %s [%s]\n", prefix, connector, emoji, slug, status)
	fmt.Printf("%s   %s\n", prefix, truncateString(node.Question, 60))

	if node.ChosenLabel != "" {
		fmt.Printf("%s   → Chose: %s\n", prefix, node.ChosenLabel)
	}

	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}
	}

	for i, child := range node.Children {
		if i > 0 || prefix != "" {
			fmt.Printf("%s │\n", childPrefix)
		}
		printChainTree(child, childPrefix, i == len(node.Children)-1)
	}
}

// embedTypeInContext adds a _type field to the context JSON.
// If context is empty, creates a new object with just the type.
// If context is an object, adds _type to it.
// If context is a non-object JSON value, wraps it in {_type: type, _value: original}.
func embedTypeInContext(context, decisionType string) string {
	if context == "" {
		// Empty context: create minimal object with just type
		obj := map[string]interface{}{"_type": decisionType}
		result, _ := json.Marshal(obj)
		return string(result)
	}

	// Try to parse existing context
	var parsed interface{}
	if err := json.Unmarshal([]byte(context), &parsed); err != nil {
		// Invalid JSON - return as-is (shouldn't happen due to earlier validation)
		return context
	}

	// If it's already an object, add _type field
	if obj, ok := parsed.(map[string]interface{}); ok {
		obj["_type"] = decisionType
		result, _ := json.Marshal(obj)
		return string(result)
	}

	// Non-object (array, string, number): wrap in object
	wrapper := map[string]interface{}{
		"_type":  decisionType,
		"_value": parsed,
	}
	result, _ := json.Marshal(wrapper)
	return string(result)
}

// extractTypeFromContext extracts the _type field from context JSON.
// Returns empty string if not found or not parseable.
func extractTypeFromContext(context string) string {
	if context == "" {
		return ""
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(context), &obj); err != nil {
		return ""
	}

	if typeVal, ok := obj["_type"].(string); ok {
		return typeVal
	}
	return ""
}

// embedSessionIDInContext adds a _session_id field to the context JSON.
// Similar to embedTypeInContext but for session tracking.
func embedSessionIDInContext(context, sessionID string) string {
	if context == "" {
		obj := map[string]interface{}{"_session_id": sessionID}
		result, _ := json.Marshal(obj)
		return string(result)
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(context), &parsed); err != nil {
		// Invalid JSON - create new object
		obj := map[string]interface{}{"_session_id": sessionID}
		result, _ := json.Marshal(obj)
		return string(result)
	}

	if obj, ok := parsed.(map[string]interface{}); ok {
		obj["_session_id"] = sessionID
		result, _ := json.Marshal(obj)
		return string(result)
	}

	// Non-object: wrap
	wrapper := map[string]interface{}{
		"_session_id": sessionID,
		"_value":      parsed,
	}
	result, _ := json.Marshal(wrapper)
	return string(result)
}

// extractSessionIDFromContext extracts the _session_id field from context JSON.
func extractSessionIDFromContext(context string) string {
	if context == "" {
		return ""
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(context), &obj); err != nil {
		return ""
	}

	if sessionID, ok := obj["_session_id"].(string); ok {
		return sessionID
	}
	return ""
}

// checkPredecessorSuggestedType looks up a predecessor decision and extracts
// any suggested successor type from its resolution rationale.
// Returns the suggested type or empty string if none found.
func checkPredecessorSuggestedType(townRoot, predecessorID string) string {
	if predecessorID == "" {
		return ""
	}

	// Look up the predecessor decision using gt decision show (has full resolution data)
	output, err := exec.Command("gt", "decision", "show", predecessorID, "--json").Output()
	if err != nil {
		return ""
	}

	var decision struct {
		Rationale string `json:"rationale"`
	}

	if err := json.Unmarshal(output, &decision); err != nil {
		return ""
	}

	rationale := decision.Rationale
	if rationale == "" {
		return ""
	}

	// Look for "→ [Suggested successor type: X]" pattern
	const prefix = "→ [Suggested successor type: "
	const suffix = "]"

	idx := strings.Index(rationale, prefix)
	if idx == -1 {
		return ""
	}

	start := idx + len(prefix)
	remaining := rationale[start:]
	endIdx := strings.Index(remaining, suffix)
	if endIdx == -1 {
		return ""
	}

	return remaining[:endIdx]
}

// validateReferencedBeads checks that all bead IDs referenced in the prompt
// and context have descriptions provided in the context's referenced_beads field.
// Returns an error if any referenced beads are missing descriptions.
func validateReferencedBeads(prompt, contextJSON string, contextMap map[string]interface{}) error {
	// Extract all bead IDs from prompt and context
	referencedIDs := beads.ExtractBeadIDsFromAll(prompt, contextJSON)
	if len(referencedIDs) == 0 {
		return nil // No beads referenced, nothing to validate
	}

	// Get the referenced_beads map from context
	var providedBeads map[string]interface{}
	if contextMap != nil {
		if rb, ok := contextMap["referenced_beads"].(map[string]interface{}); ok {
			providedBeads = rb
		}
	}

	// Check which bead IDs are missing descriptions
	var missingBeads []string
	for _, beadID := range referencedIDs {
		if providedBeads == nil {
			missingBeads = append(missingBeads, beadID)
			continue
		}

		beadInfo, exists := providedBeads[beadID]
		if !exists {
			missingBeads = append(missingBeads, beadID)
			continue
		}

		// Check that the bead has at least a title or description
		if beadMap, ok := beadInfo.(map[string]interface{}); ok {
			hasTitle := beadMap["title"] != nil && beadMap["title"] != ""
			hasDesc := beadMap["description"] != nil && beadMap["description"] != ""
			hasDescSummary := beadMap["description_summary"] != nil && beadMap["description_summary"] != ""
			if !hasTitle && !hasDesc && !hasDescSummary {
				missingBeads = append(missingBeads, beadID)
			}
		} else {
			// Entry exists but isn't a proper object
			missingBeads = append(missingBeads, beadID)
		}
	}

	if len(missingBeads) > 0 {
		style.PrintError("Missing descriptions for referenced beads: %s", strings.Join(missingBeads, ", "))
		style.PrintWarning("Hint: Use --auto-context to auto-fetch, or add to --context JSON:")
		style.PrintWarning(`  --context '{"referenced_beads": {"%s": {"title": "...", "description_summary": "..."}}}'`, missingBeads[0])
		return fmt.Errorf("referenced beads validation failed: %d bead(s) missing descriptions (use --no-bead-check to skip)", len(missingBeads))
	}

	return nil
}

// Note: Fail-then-File and successor schema validation moved to scripts:
// - validators/create-decision-fail-file.sh
// - validators/create-decision-successor.sh

