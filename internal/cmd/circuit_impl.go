package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/circuit"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCircuitStatus(_ *cobra.Command, args []string) error {
	rig := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	b, err := circuit.Load(townRoot, rig)
	if err != nil {
		return fmt.Errorf("loading circuit state: %w", err)
	}

	if circuitJSON {
		data, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	stateIcon := "✅"
	switch b.State {
	case circuit.StateOpen:
		stateIcon = "🔴"
	case circuit.StateHalfOpen:
		stateIcon = "🟡"
	}

	fmt.Printf("%s Circuit Breaker: %s — %s\n", stateIcon, rig, b.State)
	fmt.Printf("  Threshold: %d consecutive failures\n", b.Threshold)

	if b.OpenedAt != "" {
		fmt.Printf("  Opened at: %s (by: %s)\n", b.OpenedAt, b.OpenedBy)
	}
	if b.ResetAt != "" {
		fmt.Printf("  Last reset: %s (by: %s)\n", b.ResetAt, b.ResetBy)
	}

	fmt.Println()
	for _, stage := range []circuit.Stage{circuit.StageWitness, circuit.StageRefinery} {
		s := b.Stages[stage]
		if s == nil {
			continue
		}
		fmt.Printf("  %s:\n", stage)
		fmt.Printf("    Consecutive failures: %d\n", s.ConsecutiveFailures)
		if s.LastFailureAt != "" {
			fmt.Printf("    Last failure: %s — %s\n", s.LastFailureAt, s.LastFailureReason)
		}
		if s.LastSuccessAt != "" {
			fmt.Printf("    Last success: %s\n", s.LastSuccessAt)
		}
	}

	return nil
}

func runCircuitReset(_ *cobra.Command, args []string) error {
	rig := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	b, err := circuit.Load(townRoot, rig)
	if err != nil {
		return fmt.Errorf("loading circuit state: %w", err)
	}

	prevState := b.State

	actor := detectSender()
	if actor == "" {
		actor = "manual"
	}
	b.Reset(actor)

	if err := circuit.Save(townRoot, b); err != nil {
		return fmt.Errorf("saving circuit state: %w", err)
	}

	if prevState != circuit.StateClosed {
		fmt.Printf("✓ Circuit breaker for %s reset: %s → closed (by %s)\n", rig, prevState, actor)
	} else {
		fmt.Printf("Circuit breaker for %s is already closed (reset recorded)\n", rig)
	}

	return nil
}
