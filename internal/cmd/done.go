// Write completion metadata to agent bead for audit trail.
// Self-managed completion (gt-1qlg): metadata is retained for anomaly
// detection and crash recovery by witness patrol, but the witness no
// longer processes routine completions from these fields.

	fmt.Printf("\nNotifying Witness...\n")
	if agentBeadID != "" {
		completionBd := beads.New(beads.ResolveBeadsDir(cwd))
		meta := &beads.CompletionMetadata{
			ExitType:       exitType,
			MRID:           mrID,
			Branch:         branch,
			HookBead:       issueID,
			MRFailed:       mrFailed,
			CompletionTime: time.Now().UTC().Format(time.RFC3339),
		}
		if err := completionBd.UpdateAgentCompletion(agentBeadID, meta); err != nil {
			style.PrintWarning("could not write completion metadata to agent bead: %v", err)
		}
	}

	// Nudge witness via tmux (observability, not critical path).
	// Self-managed completion (gt-1qlg): witness no longer processes routine completions.
	// The nudge is kept for observability — witness logs the event but doesn't
	// need to act on it. Nudges are free (no Dolt commit).
	nudgeWitness(rigName, fmt.Sprintf("POLECAT_DONE %s exit=%s", polecatName, exitType))
	fmt.Printf("%s Witness notified of %s (via nudge)\n", style.Bold.Render("✓"), exitType)

	// Write witness notification checkpoint for resume (gt-aufru)
	if agentBeadID != "" {
		cpBd := beads.New(beads.ResolveBeadsDir(cwd))
		writeDoneCheckpoint(cpBd, agentBeadID, CheckpointWitnessNotified, "ok")
	}

	// Log done event (townlog and activity feed)
	if err := LogDone(townRoot, sender, issueID); err != nil {
		style.PrintWarning("could not log done event: %v", err)
	}
	if err := events.LogFeed(events.TypeDone, sender, events.DonePayload(issueID, branch)); err != nil {
		style.PrintWarning("could not log feed event: %v", err)
	}

	// Update agent bead state (ZFC: self-report completion)
	updateAgentStateOnDone(cwd, townRoot, exitType, issueID)

	// CRITICAL: Wait for all async mail notifications to complete BEFORE killing
	// the session. The selfKillSession call below terminates the process immediately,
	// which would skip any deferred functions. This ensures POLECAT_DONE and WORK_DONE
	// notifications are actually delivered to the witness.
	townRouter.WaitPendingNotifications()

	// Persistent polecat model (gt-hdf8): polecats transition to IDLE after completion.
	// Session stays alive, sandbox preserved, worktree synced to main for reuse.
	// "done means idle" - not "done means dead".
	isPolecat := false
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil && roleInfo.Role == RolePolecat {
		isPolecat = true

		fmt.Printf("%s Sandbox preserved for reuse (persistent polecat)\n", style.Bold.Render("✓"))

		if pushFailed || mrFailed {
			fmt.Printf("%s Work needs recovery (push or MR failed) — session preserved\n", style.Bold.Render("⚠"))
		}

		// Sync worktree to main so the polecat is ready for new assignments.
		// Phase 3 of persistent-polecat-pool: DONE→IDLE syncs to main and deletes old branch.
		// Non-fatal: if sync fails, the polecat is still IDLE and the Witness
		// or next gt sling can handle the branch state.
		if cwdAvailable && !pushFailed {
			// Remember the old branch so we can delete it after switching
			oldBranch := branch

			fmt.Printf("%s Syncing worktree to %s...\n", style.Bold.Render("→"), defaultBranch)
			if err := g.Checkout(defaultBranch); err != nil {
				style.PrintWarning("could not checkout %s: %v (worktree stays on feature branch)", defaultBranch, err)
			} else if err := g.Pull("origin", defaultBranch); err != nil {
				style.PrintWarning("could not pull %s: %v (worktree on %s but may be stale)", defaultBranch, defaultBranch, err)
			} else {
				fmt.Printf("%s Worktree synced to %s\n", style.Bold.Render("✓"), defaultBranch)
			}

			// Delete the old polecat branch (non-fatal: cleanup only).
			// This prevents stale branch accumulation from persistent polecats.
			if oldBranch != "" && oldBranch != defaultBranch && oldBranch != "master" {
				if err := g.DeleteBranch(oldBranch, true); err != nil {
					style.PrintWarning("could not delete old branch %s: %v", oldBranch, err)
				} else {
					fmt.Printf("%s Deleted old branch %s\n", style.Bold.Render("✓"), oldBranch)
				}
			}
		}

		fmt.Printf("%s Polecat transitioned to IDLE — ready for new work\n", style.Bold.Render("✓"))
	}

	fmt.Println()
	if !isPolecat {
		fmt.Printf("%s Session exiting\n", style.Bold.Render("→"))
		fmt.Printf("  Witness will handle cleanup.\n")
	}
	return nil