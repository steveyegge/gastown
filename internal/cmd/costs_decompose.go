// Package cmd — `gt costs decompose` subcommand for ka-4nl Phase 0.
//
// Iterates ~/.claude-accounts/{acct1,acct2}/projects/*/, classifies each
// session jsonl into per-axis events (cache hit, retry, /clear burn, ladder
// transitions where applicable), emits the four event streams, and prints a
// per-crew distribution table + Phase 0 close artifact (before/after/delta).
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	decomposeJSON         bool
	decomposeOutputDir    string
	decomposeAccountsRoot string
	decomposeBefore       string // ISO8601 cutoff for "before" baseline (default: 7 days ago)
	decomposeCloseArtifact bool
)

var costsDecomposeCmd = &cobra.Command{
	Use:   "decompose",
	Short: "ka-4nl Phase 0 — per-axis token-spend decomposition across crews",
	Long: `Phase 0 instrumentation for the S-E hybrid Active-Standby + Self-Pacing
spec (occultfusion/crew/scrutor/specs/2026-04-27_S-E_..._self-pacing.md §6.1).

Reads every Claude Code session jsonl under ~/.claude-accounts/{acct1,acct2}
and produces four append-only event streams:

  $CLAUDE_ACCOUNTS_ROOT/usage-ladder-events.jsonl   threshold-band crossings (AC-0.1)
  $CLAUDE_ACCOUNTS_ROOT/cache-events.jsonl          per-session-day cache_hit_pct (AC-0.3)
                                                    + per-/clear cache invalidation burn (AC-0.6)
  $CLAUDE_ACCOUNTS_ROOT/retry-events.jsonl          per-API-error turn (AC-0.5, upper-bound proxy)

Waivers (Munger hq-wisp-eabjib):
  AC-0.4 (A2 thinking_tokens) — text redacted to empty by Anthropic; emitter
    records thinking_block_count_per_turn as proxy. True token cost requires
    live API capture (out of Phase 0 read-only scope).
  AC-0.5 (A3 retry_token_cost) — upper-bound proxy (input + cache_creation
    of failed turn); conflates retry-overhead with normal request cost.

Pure-read; no behavior change. Kill switch = unregister this command.

Examples:
  gt costs decompose                                   # emit + summary
  gt costs decompose --json                            # JSON summary
  gt costs decompose --close-artifact                  # also emit Phase 0 close artifact
  gt costs decompose --accounts-root ~/.claude-accounts --before 2026-04-20T00:00:00Z`,
	RunE: runCostsDecompose,
}

func init() {
	costsCmd.AddCommand(costsDecomposeCmd)
	costsDecomposeCmd.Flags().BoolVar(&decomposeJSON, "json", false, "Output summary as JSON")
	costsDecomposeCmd.Flags().StringVar(&decomposeOutputDir, "output-dir", "",
		"Override output dir for the four event streams (default: $CLAUDE_ACCOUNTS_ROOT)")
	costsDecomposeCmd.Flags().StringVar(&decomposeAccountsRoot, "accounts-root", "",
		"Override ~/.claude-accounts root (defaults to $HOME/.claude-accounts)")
	costsDecomposeCmd.Flags().StringVar(&decomposeBefore, "before", "",
		"ISO8601 cutoff for the 'before' baseline window (default: 7 days before now)")
	costsDecomposeCmd.Flags().BoolVar(&decomposeCloseArtifact, "close-artifact", false,
		"Also emit Phase 0 close artifact (before/after/delta + per-axis distribution)")
}

// crewClass labels the four AC-0.7 sample categories.
type crewClass string

const (
	classHistoryHeavy crewClass = "history_heavy"
	classToolHeavy    crewClass = "tool_heavy"
	classLowContent   crewClass = "low_content"
	classPolecat      crewClass = "polecat"
	classOther        crewClass = "other"
)

// classifyCrew picks an AC-0.7 category from an encoded-cwd crew identifier.
// Heuristic only — Phase 0 documentation should cite this as proxy.
func classifyCrew(crew string) crewClass {
	c := strings.ToLower(crew)
	switch {
	case strings.Contains(c, "polecat"):
		return classPolecat
	case strings.Contains(c, "witness"), strings.Contains(c, "mayor"):
		return classLowContent
	case strings.Contains(c, "atlas"), strings.Contains(c, "deacon"),
		strings.Contains(c, "refinery"), strings.Contains(c, "backend"):
		return classToolHeavy
	case strings.Contains(c, "scrutor"), strings.Contains(c, "munger"),
		strings.Contains(c, "musk"), strings.Contains(c, "cmo"),
		strings.Contains(c, "researcher"):
		return classHistoryHeavy
	default:
		return classOther
	}
}

// CrewSummary is the per-crew aggregate row in the close artifact.
type CrewSummary struct {
	Crew                string    `json:"crew"`
	Class               crewClass `json:"class"`
	Account             string    `json:"account"`
	Turns               int       `json:"turns"`
	InputTokens         int       `json:"input_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	CacheReadTokens     int       `json:"cache_read_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	ThinkingBlocks      int       `json:"thinking_blocks_proxy_a2"`
	RetryEvents         int       `json:"retry_events"`
	ClearBurns          int       `json:"clear_burns"`
	CacheHitPct         float64   `json:"cache_hit_pct"`
}

// CloseArtifact mirrors the AC-0.8 before/after/delta structure that the
// Phase 0 close-bead must cite.
type CloseArtifact struct {
	GeneratedAt    string                 `json:"generated_at"`
	BeforeCutoff   string                 `json:"before_cutoff"`
	AfterCutoff    string                 `json:"after_cutoff"`
	Before         StreamCounts           `json:"before"`
	After          StreamCounts           `json:"after"`
	Delta          StreamCounts           `json:"delta"`
	PerCrew        []CrewSummary          `json:"per_crew"`
	WaiverNotes    map[string]string      `json:"waiver_notes"`
	MuskValidation MuskDecompositionCheck `json:"musk_validation"`
}

type StreamCounts struct {
	LadderEvents int `json:"usage_ladder_events"`
	CacheEvents  int `json:"cache_events"`
	RetryEvents  int `json:"retry_events"`
}

// MuskDecompositionCheck reports whether the aggregated per-axis percentages
// approach 100% (sanity check on decomposition completeness, AC-0.8). The
// thinking_pct column is the BLOCK-COUNT proxy normalized to a fraction of
// turns; per AC-0.4 waiver this is not a true token-percentage.
type MuskDecompositionCheck struct {
	HistoryPct       float64 `json:"history_pct_proxy"`        // cache_creation+input share of total
	ToolPct          float64 `json:"tool_pct_proxy"`           // output share (incl. tool_use blocks)
	ThinkingPct      float64 `json:"thinking_pct_proxy_a2"`    // thinking_blocks / turns (NOT token share)
	CacheReadPct     float64 `json:"cache_read_pct"`           // cache_read share of total
	RetryOverheadPct float64 `json:"retry_overhead_pct_proxy"` // retry_token_cost / total (upper-bound)
	TotalCheckPct    float64 `json:"total_check_pct"`          // sum of token shares (target ~100)
	Caveat           string  `json:"caveat"`
}

const muskDecompCaveat = "thinking_pct is BLOCK-COUNT proxy per AC-0.4 waiver, not a token share; sum excludes thinking and is upper-bounded by retry_overhead conflation per AC-0.5"

func runCostsDecompose(_ *cobra.Command, _ []string) error {
	root, err := resolveAccountsRoot()
	if err != nil {
		return err
	}
	outDir := decomposeOutputDir
	if outDir == "" {
		outDir = root
	}

	transcripts, err := findAllSessionTranscripts(root)
	if err != nil {
		return fmt.Errorf("walking transcripts: %w", err)
	}
	if len(transcripts) == 0 {
		return fmt.Errorf("no transcripts found under %s — accounts dir empty?", root)
	}

	var allTurns []TurnRecord
	for _, p := range transcripts {
		turns, err := parseSessionTurns(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: parsing %s: %v\n", p, err)
			continue
		}
		allTurns = append(allTurns, turns...)
	}

	// AC-0.3 + AC-0.6: cache events (session-day aggregates + clear burns)
	cacheEvents := aggregateCacheBySessionDay(allTurns)
	clearBurns := detectClearBurnsByCrew(allTurns)
	cacheEvents = append(cacheEvents, clearBurns...)

	// AC-0.5: retry events
	retryEvents := emitRetryEvents(allTurns)

	// AC-0.1: ladder transitions — Phase 0 has no live quota signal, so
	// for the calibration sample we model remaining_pct from cumulative
	// per-day burn (proxy: 100 * (1 - day_used/day_budget)). Phase 1 wires
	// in the real signal.
	ladderEvents := detectLadderTransitions(allTurns, calibrationRemainingPct(allTurns))

	if err := appendJSONL(filepath.Join(outDir, "usage-ladder-events.jsonl"), ladderEvents); err != nil {
		return fmt.Errorf("emitting ladder events: %w", err)
	}
	if err := appendJSONL(filepath.Join(outDir, "cache-events.jsonl"), cacheEvents); err != nil {
		return fmt.Errorf("emitting cache events: %w", err)
	}
	if err := appendJSONL(filepath.Join(outDir, "retry-events.jsonl"), retryEvents); err != nil {
		return fmt.Errorf("emitting retry events: %w", err)
	}

	summary := buildSummary(allTurns, ladderEvents, cacheEvents, retryEvents)

	if decomposeCloseArtifact {
		artifact := buildCloseArtifact(allTurns, ladderEvents, cacheEvents, retryEvents)
		artifactPath := filepath.Join(outDir, "phase0-close-artifact.json")
		f, err := os.Create(artifactPath)
		if err != nil {
			return fmt.Errorf("creating close artifact: %w", err)
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(artifact); err != nil {
			f.Close()
			return fmt.Errorf("encoding close artifact: %w", err)
		}
		f.Close()
		summary["close_artifact_path"] = artifactPath
	}

	if decomposeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	printDecomposeSummary(summary)
	return nil
}

// detectClearBurnsByCrew partitions turns by (account, crew) so that the
// session-tuple correlation in detectClearBurns() is per-project, not global.
func detectClearBurnsByCrew(turns []TurnRecord) []CacheEvent {
	type key struct{ account, crew string }
	groups := map[key][]TurnRecord{}
	for _, t := range turns {
		k := key{t.Account, t.Crew}
		groups[k] = append(groups[k], t)
	}
	var out []CacheEvent
	for _, g := range groups {
		out = append(out, detectClearBurns(g)...)
	}
	return out
}

// calibrationRemainingPct returns a remaining-quota approximation for Phase 0
// ladder calibration, since we don't have a real quota signal yet. We treat
// each calendar day as a budget of "max output tokens observed in any day"
// and decrement linearly; this is intentionally crude — Phase 1 calibration
// replaces it with a measured quota source.
func calibrationRemainingPct(turns []TurnRecord) func(TurnRecord) float64 {
	type key struct{ account, day string }
	dayBudget := map[key]int{}
	for _, t := range turns {
		k := key{t.Account, t.Timestamp.UTC().Format("2006-01-02")}
		dayBudget[k] += t.OutputTokens
	}
	maxBudget := 0
	for _, v := range dayBudget {
		if v > maxBudget {
			maxBudget = v
		}
	}
	if maxBudget == 0 {
		maxBudget = 1
	}
	used := map[key]int{}
	return func(t TurnRecord) float64 {
		k := key{t.Account, t.Timestamp.UTC().Format("2006-01-02")}
		used[k] += t.OutputTokens
		return 100.0 * (1.0 - float64(used[k])/float64(maxBudget))
	}
}

func resolveAccountsRoot() (string, error) {
	if decomposeAccountsRoot != "" {
		return decomposeAccountsRoot, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude-accounts"), nil
}

func buildSummary(turns []TurnRecord, ladder []LadderEvent, cache []CacheEvent, retry []RetryEvent) map[string]any {
	return map[string]any{
		"turns_total":             len(turns),
		"ladder_events_emitted":   len(ladder),
		"cache_events_emitted":    len(cache),
		"retry_events_emitted":    len(retry),
		"crews_sampled":           countCrews(turns),
		"ac_0_4_thinking_waiver":  "block-count proxy (text redacted by Anthropic, count visible)",
		"ac_0_5_retry_caveat":     retryCostCaveat,
	}
}

func countCrews(turns []TurnRecord) int {
	seen := map[string]bool{}
	for _, t := range turns {
		seen[t.Account+"/"+t.Crew] = true
	}
	return len(seen)
}

func printDecomposeSummary(s map[string]any) {
	fmt.Printf("ka-4nl Phase 0 decomposition\n")
	fmt.Printf("  turns processed:        %v\n", s["turns_total"])
	fmt.Printf("  crews sampled:          %v\n", s["crews_sampled"])
	fmt.Printf("  usage-ladder events:    %v\n", s["ladder_events_emitted"])
	fmt.Printf("  cache events:           %v\n", s["cache_events_emitted"])
	fmt.Printf("  retry events:           %v\n", s["retry_events_emitted"])
	fmt.Printf("\nWaivers (Munger hq-wisp-eabjib):\n")
	fmt.Printf("  AC-0.4: %v\n", s["ac_0_4_thinking_waiver"])
	fmt.Printf("  AC-0.5: %v\n", s["ac_0_5_retry_caveat"])
	if p, ok := s["close_artifact_path"]; ok {
		fmt.Printf("\nClose artifact: %v\n", p)
	}
}

// buildCloseArtifact assembles the AC-0.8 before/after/delta record. Before
// = events whose timestamp ≤ decomposeBefore (default: 7 days ago). After =
// the rest. Delta = after - before.
func buildCloseArtifact(turns []TurnRecord, ladder []LadderEvent, cache []CacheEvent, retry []RetryEvent) CloseArtifact {
	before := time.Now().Add(-7 * 24 * time.Hour)
	if decomposeBefore != "" {
		if t, err := time.Parse(time.RFC3339, decomposeBefore); err == nil {
			before = t
		}
	}
	bcount := StreamCounts{}
	acount := StreamCounts{}
	for _, e := range ladder {
		if eventBefore(e.Timestamp, before) {
			bcount.LadderEvents++
		} else {
			acount.LadderEvents++
		}
	}
	for _, e := range cache {
		if eventBefore(e.Timestamp, before) {
			bcount.CacheEvents++
		} else {
			acount.CacheEvents++
		}
	}
	for _, e := range retry {
		if eventBefore(e.Timestamp, before) {
			bcount.RetryEvents++
		} else {
			acount.RetryEvents++
		}
	}
	return CloseArtifact{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		BeforeCutoff:   before.UTC().Format(time.RFC3339),
		AfterCutoff:    time.Now().UTC().Format(time.RFC3339),
		Before:         bcount,
		After:          acount,
		Delta:          StreamCounts{LadderEvents: acount.LadderEvents - bcount.LadderEvents, CacheEvents: acount.CacheEvents - bcount.CacheEvents, RetryEvents: acount.RetryEvents - bcount.RetryEvents},
		PerCrew:        buildPerCrewSummary(turns, retry, cache),
		WaiverNotes:    buildWaiverNotes(),
		MuskValidation: buildMuskCheck(turns, retry),
	}
}

func eventBefore(ts string, cutoff time.Time) bool {
	if ts == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	return t.Before(cutoff)
}

func buildPerCrewSummary(turns []TurnRecord, retry []RetryEvent, cache []CacheEvent) []CrewSummary {
	type key struct{ account, crew string }
	agg := map[key]*CrewSummary{}
	for _, t := range turns {
		k := key{t.Account, t.Crew}
		s, ok := agg[k]
		if !ok {
			s = &CrewSummary{Account: t.Account, Crew: t.Crew, Class: classifyCrew(t.Crew)}
			agg[k] = s
		}
		s.Turns++
		s.InputTokens += t.InputTokens
		s.CacheCreationTokens += t.CacheCreationTokens
		s.CacheReadTokens += t.CacheReadTokens
		s.OutputTokens += t.OutputTokens
		s.ThinkingBlocks += t.ThinkingBlockCount
	}
	for _, r := range retry {
		s := agg[key{r.Account, r.Crew}]
		if s != nil {
			s.RetryEvents++
		}
	}
	for _, c := range cache {
		if c.Kind != "clear_burn" {
			continue
		}
		s := agg[key{c.Account, c.Crew}]
		if s != nil {
			s.ClearBurns++
		}
	}
	out := make([]CrewSummary, 0, len(agg))
	for _, s := range agg {
		denom := s.CacheReadTokens + s.CacheCreationTokens + s.InputTokens
		if denom > 0 {
			s.CacheHitPct = float64(s.CacheReadTokens) / float64(denom)
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Account != out[j].Account {
			return out[i].Account < out[j].Account
		}
		return out[i].Crew < out[j].Crew
	})
	return out
}

func buildWaiverNotes() map[string]string {
	return map[string]string{
		"AC-0.4_A2_thinking_tokens": "Session-jsonl persists thinking content blocks with empty 'thinking' field + signature only. Token count NOT recoverable from session data; thinking_block_count_per_turn captured as proxy. True token cost requires live API capture (out of Phase 0 read-only scope). Munger waiver: hq-wisp-eabjib OQ-1.",
		"AC-0.5_A3_retry_token_cost": retryCostCaveat + ". Munger waiver: hq-wisp-eabjib OQ-2. Phase 1 calibration may refine to incremental delta.",
		"AC-0.6_A4_clear_burn":       "/clear cache-invalidation burn captured via cross-session sessionId-tuple correlation (within-session invisible by Anthropic design — /clear opens a new sessionId). prev_session_id + prev_session_last_cache_read fields document the audit pair per Scrutor caveat.",
	}
}

func buildMuskCheck(turns []TurnRecord, retry []RetryEvent) MuskDecompositionCheck {
	var input, cacheCreate, cacheRead, output, thinking int
	for _, t := range turns {
		input += t.InputTokens
		cacheCreate += t.CacheCreationTokens
		cacheRead += t.CacheReadTokens
		output += t.OutputTokens
		thinking += t.ThinkingBlockCount
	}
	var retryCost int
	for _, r := range retry {
		retryCost += r.RetryTokenCost
	}
	total := input + cacheCreate + cacheRead + output
	if total == 0 {
		total = 1
	}
	historyPct := 100.0 * float64(input+cacheCreate) / float64(total)
	toolPct := 100.0 * float64(output) / float64(total)
	cacheReadPct := 100.0 * float64(cacheRead) / float64(total)
	retryPct := 100.0 * float64(retryCost) / float64(total)
	thinkingProxy := 0.0
	if len(turns) > 0 {
		thinkingProxy = float64(thinking) / float64(len(turns))
	}
	return MuskDecompositionCheck{
		HistoryPct:       historyPct,
		ToolPct:          toolPct,
		ThinkingPct:      thinkingProxy,
		CacheReadPct:     cacheReadPct,
		RetryOverheadPct: retryPct,
		TotalCheckPct:    historyPct + toolPct + cacheReadPct,
		Caveat:           muskDecompCaveat,
	}
}
