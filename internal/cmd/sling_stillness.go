package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

type stillnessDecision string

const (
	stillnessDecisionAct    stillnessDecision = "ACT"
	stillnessDecisionWait   stillnessDecision = "WAIT"
	stillnessDecisionRefuse stillnessDecision = "REFUSE"
)

const (
	stillnessStateVersion       = 1
	stillnessDefaultCoherence   = 100
	stillnessDissolveThreshold  = 35
	stillnessChurnWindowSeconds = int64((30 * time.Minute) / time.Second)
	stillnessPruneWindowSeconds = int64((24 * time.Hour) / time.Second)
)

type stillnessGateResult struct {
	Decision      stillnessDecision
	Reason        string
	Coherence     int
	TaskDissolved bool
	ConvoyClosed  string
}

type stillnessState struct {
	Version     int                            `json:"version"`
	Tasks       map[string]*stillnessTaskState `json:"tasks"`
	LastUpdated int64                          `json:"last_updated,omitempty"`
}

type stillnessTaskState struct {
	Coherence   int     `json:"coherence"`
	Attempts    []int64 `json:"attempts,omitempty"`
	Reversals   []int64 `json:"reversals,omitempty"`
	LastArgs    string  `json:"last_args,omitempty"`
	LastTarget  string  `json:"last_target,omitempty"`
	DissolvedAt int64   `json:"dissolved_at,omitempty"`
}

var (
	nowForStillness               = time.Now
	estimateTargetLoadFn          = estimateTargetLoad
	closeConvoyForDissolvedTaskFn = closeConvoyForDissolvedTask
	logGovernanceAuditFn          = events.LogAudit
)

func evaluateSlingStillnessGate(townRoot, beadID, target string, info *beadInfo, force bool, args string) (*stillnessGateResult, error) {
	if !shouldApplyStillnessGate() {
		return nil, nil
	}

	state, err := loadStillnessState(townRoot)
	if err != nil {
		return nil, err
	}

	now := nowForStillness().UTC().Unix()
	task := state.getTask(beadID)
	pruneStillnessTask(task, now)

	if task.DissolvedAt > 0 {
		result := &stillnessGateResult{
			Decision:  stillnessDecisionRefuse,
			Reason:    "task already dissolved by resonance decay",
			Coherence: task.Coherence,
		}
		logStillnessNoOp(beadID, normalizeStillnessTarget(target), result)
		return result, nil
	}

	normalizedTarget := normalizeStillnessTarget(target)
	if normalizedTarget == "" {
		if role := strings.ToLower(strings.TrimSpace(os.Getenv("GT_ROLE"))); role == "mayor" {
			normalizedTarget = "mayor"
		}
	}

	task.Attempts = append(task.Attempts, now)
	churn := countSince(task.Attempts, now-stillnessChurnWindowSeconds)
	reversal := force || (task.LastTarget != "" && normalizedTarget != "" && task.LastTarget != normalizedTarget)
	if reversal {
		task.Reversals = append(task.Reversals, now)
	}
	recentReversals := countSince(task.Reversals, now-stillnessChurnWindowSeconds)

	agentLoad := estimateTargetLoadFn(townRoot, normalizedTarget)
	contextExpanded := isContextExpanded(task.LastArgs, args)
	task.Coherence = decayCoherence(task.Coherence, churn, recentReversals, agentLoad, contextExpanded, reversal)

	if args != "" {
		task.LastArgs = args
	}
	if normalizedTarget != "" {
		task.LastTarget = normalizedTarget
	}
	state.LastUpdated = now

	taskAge, taskAgeKnown := taskAgeFromBead(info)
	decision, reason := decideStillnessOutcome(taskAge, taskAgeKnown, churn, agentLoad, recentReversals, task.Coherence, contextExpanded)

	result := &stillnessGateResult{
		Decision:  decision,
		Reason:    reason,
		Coherence: task.Coherence,
	}

	if decision == stillnessDecisionRefuse && task.Coherence <= stillnessDissolveThreshold {
		task.DissolvedAt = now
		result.TaskDissolved = true
		convoyID, closeErr := closeConvoyForDissolvedTaskFn(townRoot, beadID)
		if convoyID != "" {
			result.ConvoyClosed = convoyID
		}
		if closeErr != nil {
			result.Reason = fmt.Sprintf("%s (convoy close failed: %v)", result.Reason, closeErr)
		}
	}

	if err := saveStillnessState(townRoot, state); err != nil {
		return result, err
	}

	if decision != stillnessDecisionAct {
		logStillnessNoOp(beadID, normalizedTarget, result)
	}

	return result, nil
}

func shouldApplyStillnessGate() bool {
	if raw, ok := os.LookupEnv("GT_STILLNESS_GATE"); ok {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			if isProductionGovernanceEnv() {
				logStillnessGateDisableBlocked(strings.TrimSpace(raw))
				break
			}
			return false
		}
	}

	return strings.EqualFold(strings.TrimSpace(os.Getenv("GT_ROLE")), "mayor")
}

func stillnessStatePath(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "stillness-state.json")
}

func loadStillnessState(townRoot string) (*stillnessState, error) {
	state := &stillnessState{
		Version: stillnessStateVersion,
		Tasks:   make(map[string]*stillnessTaskState),
	}

	if townRoot == "" {
		return state, nil
	}

	path := stillnessStatePath(townRoot)
	data, err := os.ReadFile(path) //nolint:gosec // path is internally derived from townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, fmt.Errorf("reading stillness state: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return state, nil
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("parsing stillness state: %w", err)
	}
	if state.Tasks == nil {
		state.Tasks = make(map[string]*stillnessTaskState)
	}
	if state.Version == 0 {
		state.Version = stillnessStateVersion
	}

	return state, nil
}

func saveStillnessState(townRoot string, state *stillnessState) error {
	if townRoot == "" || state == nil {
		return nil
	}

	path := stillnessStatePath(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating stillness state dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling stillness state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // operational state, no secrets
		return fmt.Errorf("writing stillness state: %w", err)
	}

	return nil
}

func (s *stillnessState) getTask(beadID string) *stillnessTaskState {
	if s.Tasks == nil {
		s.Tasks = make(map[string]*stillnessTaskState)
	}
	task, ok := s.Tasks[beadID]
	if !ok {
		task = &stillnessTaskState{
			Coherence: stillnessDefaultCoherence,
		}
		s.Tasks[beadID] = task
	}
	if task.Coherence == 0 && task.DissolvedAt == 0 && len(task.Attempts) == 0 && len(task.Reversals) == 0 {
		task.Coherence = stillnessDefaultCoherence
	}
	if task.Coherence < 0 || task.Coherence > stillnessDefaultCoherence {
		task.Coherence = stillnessDefaultCoherence
	}
	return task
}

func pruneStillnessTask(task *stillnessTaskState, nowUnix int64) {
	if task == nil {
		return
	}
	cutoff := nowUnix - stillnessPruneWindowSeconds
	task.Attempts = filterTimestamps(task.Attempts, cutoff)
	task.Reversals = filterTimestamps(task.Reversals, cutoff)
}

func filterTimestamps(values []int64, cutoff int64) []int64 {
	if len(values) == 0 {
		return values
	}
	out := values[:0]
	for _, v := range values {
		if v >= cutoff {
			out = append(out, v)
		}
	}
	return out
}

func countSince(values []int64, since int64) int {
	count := 0
	for _, v := range values {
		if v >= since {
			count++
		}
	}
	return count
}

func normalizeStillnessTarget(target string) string {
	t := strings.TrimSpace(strings.TrimSuffix(target, "/"))
	if t == "." {
		return ""
	}
	return t
}

func taskAgeFromBead(info *beadInfo) (time.Duration, bool) {
	if info == nil || info.CreatedAt == "" {
		return 0, false
	}
	createdAt, err := parseBDTimestamp(info.CreatedAt)
	if err != nil {
		return 0, false
	}
	return nowForStillness().Sub(createdAt), true
}

func parseBDTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp format")
}

func isContextExpanded(previousArgs, currentArgs string) bool {
	previousArgs = strings.TrimSpace(previousArgs)
	currentArgs = strings.TrimSpace(currentArgs)
	if previousArgs == "" || currentArgs == "" || previousArgs == currentArgs {
		return false
	}
	if len(currentArgs) > len(previousArgs)+32 {
		return true
	}
	return !strings.Contains(currentArgs, previousArgs) && len(currentArgs) > len(previousArgs)
}

func decayCoherence(current, churn, recentReversals, agentLoad int, contextExpanded, reversal bool) int {
	if current < 0 || current > stillnessDefaultCoherence {
		current = stillnessDefaultCoherence
	}

	decay := 0
	if churn > 1 {
		decay += (churn - 1) * 4
	}
	if recentReversals > 0 {
		decay += recentReversals * 10
	}
	if reversal {
		decay += 6
	}
	if contextExpanded {
		decay += 8
	}
	if agentLoad >= 3 {
		decay += 6
	}

	next := current - decay
	if next < 0 {
		next = 0
	}
	if next > current {
		next = current
	}
	return next
}

func decideStillnessOutcome(taskAge time.Duration, ageKnown bool, churn, agentLoad, recentReversals, coherence int, contextExpanded bool) (stillnessDecision, string) {
	if coherence <= stillnessDissolveThreshold {
		return stillnessDecisionRefuse, fmt.Sprintf("coherence %d <= %d", coherence, stillnessDissolveThreshold)
	}
	if recentReversals >= 3 {
		return stillnessDecisionRefuse, fmt.Sprintf("recent reversals %d >= 3", recentReversals)
	}

	// Default bias is WAIT: score must become clearly positive to ACT.
	score := -1
	ageLabel := taskAge.Round(time.Minute).String()

	if ageKnown {
		switch {
		case taskAge >= 6*time.Hour:
			score += 2
		case taskAge >= 1*time.Hour:
			score += 1
		}
	} else {
		// Missing/invalid age is ambiguous; keep the default WAIT bias.
		score -= 1
		ageLabel = "unknown"
	}

	switch {
	case churn == 0:
		score += 2
	case churn == 1:
		score += 1
	case churn >= 3:
		score -= 2
	}

	if agentLoad >= 0 {
		switch {
		case agentLoad <= 1:
			score += 1
		case agentLoad >= 3:
			score -= 2
		}
	}

	if recentReversals == 0 {
		score += 1
	}
	if recentReversals >= 2 {
		score -= 3
	}
	if contextExpanded {
		score -= 1
	}

	if score >= 2 {
		return stillnessDecisionAct, fmt.Sprintf("score=%d age=%s churn=%d load=%d reversals=%d coherence=%d",
			score, ageLabel, churn, agentLoad, recentReversals, coherence)
	}

	return stillnessDecisionWait, fmt.Sprintf("score=%d age=%s churn=%d load=%d reversals=%d coherence=%d",
		score, ageLabel, churn, agentLoad, recentReversals, coherence)
}

func estimateTargetLoad(townRoot, target string) int {
	if townRoot == "" || target == "" {
		return -1
	}

	cmd := exec.Command("bd", "list", "--status=hooked", "--json", "--limit=0")
	cmd.Dir = townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return -1
	}

	var issues []struct {
		Assignee string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return -1
	}

	count := 0
	for _, issue := range issues {
		if matchStillnessTarget(issue.Assignee, target) {
			count++
		}
	}
	return count
}

func matchStillnessTarget(assignee, target string) bool {
	assignee = strings.TrimSpace(strings.TrimSuffix(assignee, "/"))
	target = strings.TrimSpace(strings.TrimSuffix(target, "/"))
	if assignee == "" || target == "" {
		return false
	}

	if target == "deacon/dogs" {
		return strings.HasPrefix(assignee, "deacon/dogs/")
	}

	if strings.Contains(target, "/") {
		return assignee == target
	}

	return strings.HasPrefix(assignee, target+"/")
}

func closeConvoyForDissolvedTask(townRoot, beadID string) (string, error) {
	if townRoot == "" || beadID == "" {
		return "", nil
	}

	convoyID := isTrackedByConvoy(beadID)
	if convoyID == "" {
		return "", nil
	}

	townBeads := filepath.Join(townRoot, ".beads")
	showCmd := exec.Command("bd", "show", convoyID, "--json")
	showCmd.Dir = townBeads
	var showOut bytes.Buffer
	showCmd.Stdout = &showOut
	if err := showCmd.Run(); err == nil {
		var convoys []struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(showOut.Bytes(), &convoys); err == nil && len(convoys) > 0 {
			if strings.EqualFold(strings.TrimSpace(convoys[0].Status), "closed") {
				return convoyID, nil
			}
		}
	}

	reason := fmt.Sprintf("Resonance decay: coherence dropped below threshold for %s", beadID)
	closeCmd := exec.Command("bd", "close", convoyID, "-r", reason)
	closeCmd.Dir = townBeads
	if err := closeCmd.Run(); err != nil {
		return convoyID, err
	}
	return convoyID, nil
}

func logStillnessNoOp(beadID, target string, result *stillnessGateResult) {
	if result == nil {
		return
	}

	actor := detectActor()
	if actor == "" || actor == "unknown" {
		actor = "mayor"
	}

	payload := map[string]interface{}{
		"bead":      beadID,
		"target":    target,
		"decision":  string(result.Decision),
		"reason":    result.Reason,
		"coherence": result.Coherence,
	}
	if result.TaskDissolved {
		payload["task_dissolved"] = true
	}
	if result.ConvoyClosed != "" {
		payload["convoy_closed"] = result.ConvoyClosed
	}

	_ = events.LogFeed(events.TypeNoOp, actor, payload)
}

func logStillnessGateDisableBlocked(requested string) {
	actor := detectActor()
	if actor == "" || actor == "unknown" {
		actor = "mayor"
	}

	payload := map[string]interface{}{
		"schema_version": 1,
		"phase":          "stillness_disable_blocked",
		"requested":      strings.TrimSpace(requested),
		"governance_env": strings.TrimSpace(os.Getenv("GT_GOVERNANCE_ENV")),
		"reason":         "GT_STILLNESS_GATE disable override ignored in production",
	}

	_ = logGovernanceAuditFn(events.TypeAnchorHealthGate, actor, payload)
}
