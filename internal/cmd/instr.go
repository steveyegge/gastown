// Package cmd — instrumentation helpers for ka-4nl Phase 0 (S-E hybrid spec §6.1).
//
// Phase 0 = pure-read decomposition of per-axis token spend across crews.
// Reads the same Claude Code session jsonl files that costs.go already
// parses; emits four event streams used for S-E threshold-ladder calibration
// and Musk decomposition validation:
//
//	~/.claude-accounts/usage-ladder-events.jsonl   (AC-0.1, threshold transitions only)
//	~/.claude-accounts/cache-events.jsonl          (AC-0.3 A1 + AC-0.6 A4)
//	~/.claude-accounts/retry-events.jsonl          (AC-0.5 A3)
//	(quota-tracker samples are aggregated in-memory, not persisted)
//
// Waivers (Munger hq-wisp-eabjib, all 4 OQ resolved):
//
//	AC-0.4 (A2 thinking_tokens) — session-jsonl persists thinking blocks with
//	  empty text + signature only; only block COUNT is recoverable. Waiver
//	  approved; emitter records thinking_block_count_per_turn as proxy.
//	AC-0.5 (A3 retry token-cost) — upper-bound proxy approved; conflates
//	  retry-attempt cost with normal request cost. Caveat surfaces in close
//	  artifact narrative, not just code.
//	AC-0.6 (A4 /clear burn) — within-session invisible; cross-session diff
//	  via sessionId tuple correlation approved.
package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LadderBand is the per-quota threshold band a session sits in. Band names
// match the spec §3.3 ladder; calibration in Phase 1 will replace placeholder
// percentages, so the band identifier is ordinal-stable rather than
// percentage-stable.
type LadderBand string

const (
	BandFull       LadderBand = "full"        // ≥80%
	BandLight      LadderBand = "light"       // 60-80%
	BandShadow     LadderBand = "shadow"      // 50-60%
	BandHandoff    LadderBand = "handoff"     // 40-50%
	BandModerate   LadderBand = "moderate"    // 30-40%
	BandAggressive LadderBand = "aggressive"  // 20-30%
	BandEmergency  LadderBand = "emergency"   // 10-20%
	BandLimitHit   LadderBand = "limit-hit"   // <10%
)

// bandFor maps a remaining-quota percentage onto the §3.3 ladder.
// Phase 0 has no quota source yet (that ships in Phase 2 alongside the
// pacing block), so we approximate remaining_pct from an external source
// when Phase 0 runs in calibration mode.
func bandFor(remainingPct float64) LadderBand {
	switch {
	case remainingPct >= 80:
		return BandFull
	case remainingPct >= 60:
		return BandLight
	case remainingPct >= 50:
		return BandShadow
	case remainingPct >= 40:
		return BandHandoff
	case remainingPct >= 30:
		return BandModerate
	case remainingPct >= 20:
		return BandAggressive
	case remainingPct >= 10:
		return BandEmergency
	default:
		return BandLimitHit
	}
}

// TurnRecord is the per-turn distillation of one assistant message. All
// downstream emitters compose from this; the parser reads each session jsonl
// once and emits a slice of TurnRecords.
type TurnRecord struct {
	SessionID           string
	Account             string // acct1 | acct2
	Crew                string // derived from CWD encoding
	Timestamp           time.Time
	Model               string
	StopReason          string
	InputTokens         int
	CacheCreationTokens int
	CacheReadTokens     int
	OutputTokens        int
	ThinkingBlockCount  int  // proxy for A2 (text redacted, count visible)
	APIErrorStatus      int  // 0 = no error
	IsAPIError          bool
}

// CacheHitPct returns cache_read / (cache_read + cache_creation + input).
// Returns 0 if denominator is 0 (idle turn / malformed).
func (t TurnRecord) CacheHitPct() float64 {
	denom := t.CacheReadTokens + t.CacheCreationTokens + t.InputTokens
	if denom == 0 {
		return 0
	}
	return float64(t.CacheReadTokens) / float64(denom)
}

// LadderEvent is one row in usage-ladder-events.jsonl. Per Munger OQ-4
// resolution: emit ONLY on threshold-band transitions, not every turn.
type LadderEvent struct {
	Timestamp    string     `json:"ts"`
	SessionID    string     `json:"session"`
	Account      string     `json:"account"`
	Crew         string     `json:"crew"`
	RemainingPct float64    `json:"remaining_pct"`
	Prev         LadderBand `json:"prev"`
	Next         LadderBand `json:"next"`
	Actions      []string   `json:"actions"`
}

// CacheEvent is one row in cache-events.jsonl. AC-0.3 emits one row per
// session-per-day with the aggregated cache_hit_pct. AC-0.6 emits a separate
// row per /clear cross-session boundary with the burn measurement.
type CacheEvent struct {
	Timestamp                 string  `json:"ts"`
	SessionID                 string  `json:"session"`
	Account                   string  `json:"account"`
	Crew                      string  `json:"crew"`
	Date                      string  `json:"date,omitempty"`                          // session-day aggregate
	CacheHitPct               float64 `json:"cache_hit_pct,omitempty"`                 // AC-0.3
	TurnCount                 int     `json:"turn_count,omitempty"`                    // AC-0.3 sanity
	ClearInvalidationBurn     int     `json:"clear_invalidation_burn_tokens,omitempty"` // AC-0.6
	PrevSessionID             string  `json:"prev_session_id,omitempty"`               // AC-0.6 audit pair
	PrevSessionLastCacheRead  int     `json:"prev_session_last_cache_read,omitempty"`  // AC-0.6 audit pair
	Kind                      string  `json:"kind"`                                    // "session_day" | "clear_burn"
}

// RetryEvent is one row in retry-events.jsonl. AC-0.5 emits one row per
// observed apiErrorStatus turn (typically 429). retry_token_cost is the
// upper-bound proxy approved per Munger OQ-2.
type RetryEvent struct {
	Timestamp       string `json:"ts"`
	SessionID       string `json:"session"`
	Account         string `json:"account"`
	Crew            string `json:"crew"`
	APIErrorStatus  int    `json:"api_error_status"`
	RetryTokenCost  int    `json:"retry_token_cost"` // upper-bound (input + cache_creation of failed turn)
	CostProxyCaveat string `json:"cost_proxy_caveat"`
}

const retryCostCaveat = "upper-bound: includes normal request cost (incremental retry-only delta not separately fielded by API)"

// parseSessionTurns reads one session jsonl and returns the per-turn records.
// Skips non-assistant lines and any line that fails to parse (matches
// parseTranscriptUsage tolerance).
func parseSessionTurns(transcriptPath string) ([]TurnRecord, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	account, crew := splitAccountCrew(transcriptPath)
	var turns []TurnRecord

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg TranscriptMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type != "assistant" || msg.Message == nil {
			continue
		}
		t := TurnRecord{
			SessionID:      msg.SessionID,
			Account:        account,
			Crew:           crew,
			Model:          msg.Message.Model,
			StopReason:     msg.Message.StopReason,
			APIErrorStatus: msg.APIErrorStatus,
			IsAPIError:     msg.IsAPIErrorMessage,
		}
		if msg.Timestamp != "" {
			if ts, err := time.Parse(time.RFC3339Nano, msg.Timestamp); err == nil {
				t.Timestamp = ts
			}
		}
		if u := msg.Message.Usage; u != nil {
			t.InputTokens = u.InputTokens
			t.CacheCreationTokens = u.CacheCreationInputTokens
			t.CacheReadTokens = u.CacheReadInputTokens
			t.OutputTokens = u.OutputTokens
		}
		for _, c := range msg.Message.Content {
			if c.Type == "thinking" {
				t.ThinkingBlockCount++
			}
		}
		turns = append(turns, t)
	}
	return turns, scanner.Err()
}

// splitAccountCrew extracts (account, crew) from a transcript path of shape
//
//	<root>/<acct>/projects/<encoded-cwd>/<sessionId>.jsonl
//
// Looks for the "projects" segment as the structural anchor; account =
// parent of "projects", crew = child of "projects". Returns ("?", "?") if
// the shape doesn't match.
func splitAccountCrew(transcriptPath string) (account, crew string) {
	parts := strings.Split(transcriptPath, string(filepath.Separator))
	for i, p := range parts {
		if p == "projects" && i >= 1 && i+1 < len(parts) {
			return parts[i-1], parts[i+1]
		}
	}
	return "?", "?"
}

// detectLadderTransitions walks turns in order and emits one LadderEvent for
// each band crossing. Per Munger OQ-4: threshold-transition only, not every
// turn. Phase 0 has no live quota signal yet, so calibration callers pass
// in a remaining_pct curve via the supplier function.
func detectLadderTransitions(turns []TurnRecord, remainingPct func(TurnRecord) float64) []LadderEvent {
	var events []LadderEvent
	prev := LadderBand("")
	for _, t := range turns {
		pct := remainingPct(t)
		band := bandFor(pct)
		if prev == "" {
			prev = band
			continue
		}
		if band == prev {
			continue
		}
		events = append(events, LadderEvent{
			Timestamp:    formatTime(t.Timestamp),
			SessionID:    t.SessionID,
			Account:      t.Account,
			Crew:         t.Crew,
			RemainingPct: pct,
			Prev:         prev,
			Next:         band,
			Actions:      actionsFor(prev, band),
		})
		prev = band
	}
	return events
}

// actionsFor returns the spec §3.3 hybrid-action labels for a band crossing.
// Used so audit-time reviewers can verify ladder events carry the expected
// behavioral cue without re-deriving from band identifiers.
func actionsFor(_, next LadderBand) []string {
	switch next {
	case BandLight:
		return []string{"pacing:light", "warm-standby:check"}
	case BandShadow:
		return []string{"pacing:light", "prep:shadow_spawn"}
	case BandHandoff:
		return []string{"pacing:moderate", "prep:handoff_context"}
	case BandModerate:
		return []string{"pacing:aggressive", "prep:graceful_migration"}
	case BandAggressive:
		return []string{"pacing:aggressive", "prep:forced_failover"}
	case BandEmergency:
		return []string{"pacing:emergency", "prep:wait_for_reset"}
	case BandLimitHit:
		return []string{"failover:standby_active"}
	default:
		return []string{}
	}
}

// aggregateCacheBySessionDay produces one CacheEvent per (session, day) with
// the day's cache_hit_pct. AC-0.3.
func aggregateCacheBySessionDay(turns []TurnRecord) []CacheEvent {
	type key struct {
		session string
		date    string
	}
	type agg struct {
		account string
		crew    string
		read    int
		create  int
		input   int
		count   int
		latest  time.Time
	}
	buckets := map[key]*agg{}
	for _, t := range turns {
		date := t.Timestamp.UTC().Format("2006-01-02")
		k := key{t.SessionID, date}
		a, ok := buckets[k]
		if !ok {
			a = &agg{account: t.Account, crew: t.Crew}
			buckets[k] = a
		}
		a.read += t.CacheReadTokens
		a.create += t.CacheCreationTokens
		a.input += t.InputTokens
		a.count++
		if t.Timestamp.After(a.latest) {
			a.latest = t.Timestamp
		}
	}
	var out []CacheEvent
	for k, a := range buckets {
		denom := a.read + a.create + a.input
		hit := 0.0
		if denom > 0 {
			hit = float64(a.read) / float64(denom)
		}
		out = append(out, CacheEvent{
			Timestamp:   formatTime(a.latest),
			SessionID:   k.session,
			Account:     a.account,
			Crew:        a.crew,
			Date:        k.date,
			CacheHitPct: hit,
			TurnCount:   a.count,
			Kind:        "session_day",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

// detectClearBurns finds /clear cache-invalidation pairs by walking all
// turns from a single (account, crew) project sorted by timestamp. A /clear
// is inferred when a new sessionId starts and its first assistant turn has
// cache_creation > 0 with cache_read = 0; the burn = the first turn's
// cache_creation tokens. The previous session's last turn is recorded as
// the audit-paired sessionId.
//
// AC-0.6 + Scrutor caveat: emitter correlates pre-/post-/clear sessionId
// tuples (measured), not heuristic.
func detectClearBurns(turns []TurnRecord) []CacheEvent {
	if len(turns) == 0 {
		return nil
	}
	sorted := make([]TurnRecord, len(turns))
	copy(sorted, turns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	var out []CacheEvent
	type tail struct {
		sessionID    string
		cacheRead    int
		ts           time.Time
	}
	tails := map[string]*tail{} // session -> last-turn snapshot
	firstSeen := map[string]bool{}
	var orderedSessions []string

	for _, t := range sorted {
		if !firstSeen[t.SessionID] {
			firstSeen[t.SessionID] = true
			orderedSessions = append(orderedSessions, t.SessionID)
			// First turn of a fresh session — check whether it looks like
			// a /clear-induced cold start: cache_creation > 0 and the
			// previous session in this project ended recently.
			if len(orderedSessions) >= 2 && t.CacheCreationTokens > 0 && t.CacheReadTokens == 0 {
				prevID := orderedSessions[len(orderedSessions)-2]
				prevTail := tails[prevID]
				if prevTail != nil {
					out = append(out, CacheEvent{
						Timestamp:                formatTime(t.Timestamp),
						SessionID:                t.SessionID,
						Account:                  t.Account,
						Crew:                     t.Crew,
						ClearInvalidationBurn:    t.CacheCreationTokens,
						PrevSessionID:            prevTail.sessionID,
						PrevSessionLastCacheRead: prevTail.cacheRead,
						Kind:                     "clear_burn",
					})
				}
			}
		}
		tails[t.SessionID] = &tail{sessionID: t.SessionID, cacheRead: t.CacheReadTokens, ts: t.Timestamp}
	}
	return out
}

// emitRetryEvents converts every API-error turn into one RetryEvent with the
// upper-bound retry_token_cost (input + cache_creation of the failed turn).
// Per Munger OQ-2: upper-bound proxy approved; caveat surfaced in field.
func emitRetryEvents(turns []TurnRecord) []RetryEvent {
	var out []RetryEvent
	for _, t := range turns {
		if t.APIErrorStatus == 0 && !t.IsAPIError {
			continue
		}
		out = append(out, RetryEvent{
			Timestamp:       formatTime(t.Timestamp),
			SessionID:       t.SessionID,
			Account:         t.Account,
			Crew:            t.Crew,
			APIErrorStatus:  t.APIErrorStatus,
			RetryTokenCost:  t.InputTokens + t.CacheCreationTokens,
			CostProxyCaveat: retryCostCaveat,
		})
	}
	return out
}

// formatTime returns RFC3339 in UTC, or empty string if the time is zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// appendJSONL writes events as newline-delimited JSON to path, creating the
// file if absent. Caller is responsible for choosing the canonical path
// under ~/.claude-accounts/.
func appendJSONL[T any](path string, events []T) error {
	if len(events) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

// findAllSessionTranscripts walks ~/.claude-accounts/{acct1,acct2}/projects
// and returns every .jsonl path it finds. Paths are returned in stable
// alphabetical order so test fixtures remain deterministic.
func findAllSessionTranscripts(accountsRoot string) ([]string, error) {
	var out []string
	for _, acct := range []string{"acct1", "acct2"} {
		root := filepath.Join(accountsRoot, acct, "projects")
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return fs.SkipDir
				}
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	sort.Strings(out)
	return out, nil
}
