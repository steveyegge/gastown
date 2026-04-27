package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture builds a synthetic session jsonl in tmp covering the four
// observable axes for the spike: cache hit vs miss, retry (429), thinking
// block (text redacted, count visible), and a session boundary that should
// trip the /clear cross-session burn detector.
func writeFixture(t *testing.T, dir, account, crew string) {
	t.Helper()
	root := filepath.Join(dir, account, "projects", crew)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Session A: 3 turns. Turn 1 = cache miss + thinking. Turn 2 = cache
	// hit. Turn 3 = retry 429.
	a := filepath.Join(root, "session-a.jsonl")
	mustWriteLines(t, a,
		assistantTurn("session-a", "2026-04-25T01:00:00Z", 100, 5000, 0, 200, 1, 0, false, "tool_use"),
		assistantTurn("session-a", "2026-04-25T01:01:00Z", 50, 0, 5100, 150, 0, 0, false, "tool_use"),
		assistantTurn("session-a", "2026-04-25T01:02:00Z", 75, 0, 5200, 0, 0, 429, true, "end_turn"),
	)

	// Session B: same project, started later. First turn has cache_creation
	// > 0 + cache_read = 0 → /clear-burn signature.
	b := filepath.Join(root, "session-b.jsonl")
	mustWriteLines(t, b,
		assistantTurn("session-b", "2026-04-25T02:00:00Z", 80, 7777, 0, 220, 2, 0, false, "tool_use"),
		assistantTurn("session-b", "2026-04-25T02:01:00Z", 40, 0, 7800, 100, 0, 0, false, "end_turn"),
	)
}

func assistantTurn(sessionID, ts string, input, cacheCreate, cacheRead, output, thinkingBlocks, errStatus int, isErr bool, stop string) string {
	body := map[string]any{
		"type":      "assistant",
		"sessionId": sessionID,
		"timestamp": ts,
		"message": map[string]any{
			"model":       "claude-opus-4-7",
			"role":        "assistant",
			"stop_reason": stop,
			"usage": map[string]any{
				"input_tokens":                input,
				"cache_creation_input_tokens": cacheCreate,
				"cache_read_input_tokens":     cacheRead,
				"output_tokens":               output,
			},
			"content": buildContent(thinkingBlocks),
		},
	}
	if errStatus != 0 {
		body["apiErrorStatus"] = errStatus
	}
	if isErr {
		body["isApiErrorMessage"] = true
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func buildContent(thinking int) []map[string]any {
	out := make([]map[string]any, 0, thinking+1)
	for i := 0; i < thinking; i++ {
		out = append(out, map[string]any{"type": "thinking", "thinking": "", "signature": "redacted"})
	}
	out = append(out, map[string]any{"type": "tool_use"})
	return out
}

func mustWriteLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func TestParseSessionTurnsExtractsAllAxes(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "acct1", "-test-crew-atlas")

	turns, err := parseSessionTurns(filepath.Join(dir, "acct1", "projects", "-test-crew-atlas", "session-a.jsonl"))
	if err != nil {
		t.Fatalf("parseSessionTurns: %v", err)
	}
	if len(turns) != 3 {
		t.Fatalf("want 3 turns, got %d", len(turns))
	}
	if turns[0].ThinkingBlockCount != 1 {
		t.Errorf("turn0 thinking blocks: want 1, got %d", turns[0].ThinkingBlockCount)
	}
	if turns[0].Account != "acct1" || turns[0].Crew != "-test-crew-atlas" {
		t.Errorf("path-derived account/crew wrong: %q / %q", turns[0].Account, turns[0].Crew)
	}
	if turns[2].APIErrorStatus != 429 || !turns[2].IsAPIError {
		t.Errorf("turn2 should be 429 retry, got status=%d isErr=%v", turns[2].APIErrorStatus, turns[2].IsAPIError)
	}
	// Cache hit on turn 2 (cache_read 5100, cache_creation 0, input 50).
	got := turns[1].CacheHitPct()
	if got < 0.95 {
		t.Errorf("turn1 cache hit pct: want >0.95, got %.3f", got)
	}
}

func TestEmitRetryEventsCarriesUpperBoundCaveat(t *testing.T) {
	turns := []TurnRecord{
		{SessionID: "s", APIErrorStatus: 429, InputTokens: 100, CacheCreationTokens: 5000},
		{SessionID: "s", InputTokens: 50}, // not a retry
	}
	events := emitRetryEvents(turns)
	if len(events) != 1 {
		t.Fatalf("want 1 retry event, got %d", len(events))
	}
	if events[0].RetryTokenCost != 5100 {
		t.Errorf("retry_token_cost: want 5100 (input+cache_creation), got %d", events[0].RetryTokenCost)
	}
	if !strings.Contains(events[0].CostProxyCaveat, "upper-bound") {
		t.Errorf("caveat must explicitly say upper-bound (Munger OQ-2 + Scrutor surfaced-not-buried check), got %q", events[0].CostProxyCaveat)
	}
}

func TestDetectClearBurnsCorrelatesSessionTuples(t *testing.T) {
	now := time.Now().UTC()
	turns := []TurnRecord{
		{SessionID: "old", Account: "acct1", Crew: "c", Timestamp: now.Add(-2 * time.Minute), CacheReadTokens: 14000, CacheCreationTokens: 0, InputTokens: 50},
		{SessionID: "old", Account: "acct1", Crew: "c", Timestamp: now.Add(-1 * time.Minute), CacheReadTokens: 14500, CacheCreationTokens: 0, InputTokens: 60},
		// New session post-/clear: cache_creation > 0, cache_read == 0
		{SessionID: "new", Account: "acct1", Crew: "c", Timestamp: now, CacheReadTokens: 0, CacheCreationTokens: 9999, InputTokens: 100},
	}
	burns := detectClearBurns(turns)
	if len(burns) != 1 {
		t.Fatalf("want 1 burn event, got %d", len(burns))
	}
	if burns[0].ClearInvalidationBurn != 9999 {
		t.Errorf("burn tokens: want 9999, got %d", burns[0].ClearInvalidationBurn)
	}
	if burns[0].PrevSessionID != "old" {
		t.Errorf("prev_session_id audit pair: want 'old', got %q", burns[0].PrevSessionID)
	}
	if burns[0].PrevSessionLastCacheRead != 14500 {
		t.Errorf("prev_session_last_cache_read: want 14500 (last turn of old), got %d", burns[0].PrevSessionLastCacheRead)
	}
	if burns[0].Kind != "clear_burn" {
		t.Errorf("kind: want clear_burn, got %q", burns[0].Kind)
	}
}

func TestAggregateCacheBySessionDayProducesOneRowPerPair(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	turns := []TurnRecord{
		{SessionID: "s1", Account: "a1", Crew: "c", Timestamp: now, CacheReadTokens: 9000, CacheCreationTokens: 0, InputTokens: 100},
		{SessionID: "s1", Account: "a1", Crew: "c", Timestamp: now.Add(time.Minute), CacheReadTokens: 9500, CacheCreationTokens: 0, InputTokens: 50},
		// different day
		{SessionID: "s1", Account: "a1", Crew: "c", Timestamp: now.Add(24 * time.Hour), CacheReadTokens: 5000, CacheCreationTokens: 1000, InputTokens: 200},
	}
	events := aggregateCacheBySessionDay(turns)
	if len(events) != 2 {
		t.Fatalf("want 2 session-day rows, got %d", len(events))
	}
	for _, e := range events {
		if e.Kind != "session_day" {
			t.Errorf("kind: want session_day, got %q", e.Kind)
		}
	}
}

func TestDetectLadderTransitionsEmitsOnlyOnBandCrossings(t *testing.T) {
	// Three turns that all sit in BandFull → 0 events expected.
	turns := []TurnRecord{
		{SessionID: "s", Timestamp: time.Now()},
		{SessionID: "s", Timestamp: time.Now().Add(time.Second)},
		{SessionID: "s", Timestamp: time.Now().Add(2 * time.Second)},
	}
	pcts := []float64{95, 90, 88}
	i := 0
	supplier := func(_ TurnRecord) float64 { p := pcts[i]; i++; return p }
	events := detectLadderTransitions(turns, supplier)
	if len(events) != 0 {
		t.Errorf("same band → 0 events; got %d", len(events))
	}
	// Now cross from full → light → shadow.
	i = 0
	pcts = []float64{95, 70, 55}
	events = detectLadderTransitions(turns, supplier)
	if len(events) != 2 {
		t.Errorf("two crossings → 2 events; got %d", len(events))
	}
	if events[0].Next != BandLight || events[1].Next != BandShadow {
		t.Errorf("crossing sequence wrong: %v → %v", events[0].Next, events[1].Next)
	}
}

func TestFindAllSessionTranscriptsIsStableSorted(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "acct1", "-test-crew-x")
	writeFixture(t, dir, "acct2", "-test-crew-y")
	paths, err := findAllSessionTranscripts(dir)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(paths) != 4 {
		t.Fatalf("want 4 jsonl files, got %d: %v", len(paths), paths)
	}
	// Stable sort means acct1 paths precede acct2 paths.
	if !strings.Contains(paths[0], "acct1") {
		t.Errorf("first path should be from acct1, got %s", paths[0])
	}
}

func TestClassifyCrewMatchesAC07Categories(t *testing.T) {
	cases := map[string]crewClass{
		"-home-karuna-gt-occultfusion-crew-atlas":   classToolHeavy,
		"-home-karuna-gt-karuna-crew-munger":        classHistoryHeavy,
		"-home-karuna-gt-gastown-witness":           classLowContent,
		"-home-karuna-gt-gastown-polecats-slit":     classPolecat,
		"-home-karuna-gt-something-unknown":         classOther,
	}
	for in, want := range cases {
		if got := classifyCrew(in); got != want {
			t.Errorf("classifyCrew(%q) = %q, want %q", in, got, want)
		}
	}
}
