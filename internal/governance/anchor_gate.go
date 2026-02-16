package governance

import (
	"bufio"
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	governanceStateVersion = 1
	freezeArtifactVersion  = 1
	governanceEventSchema  = 1

	monitoringNormal    = "NORMAL"
	monitoringEscalated = "ESCALATED"
)

var (
	logAuditEventFn = events.LogAudit
	logFeedEventFn  = events.LogFeed
	governanceMutex sync.Mutex
)

// SystemMode controls whether promotion lanes are allowed to run.
type SystemMode string

const (
	SystemModeNormal       SystemMode = "NORMAL"
	SystemModeAnchorFreeze SystemMode = "ANCHOR_FREEZE"
	SystemModeQuarantine   SystemMode = "QUARANTINE"
	SystemModeBreakGlass   SystemMode = "BREAK_GLASS"
)

// AnchorGateStatus is the only outward gate state exposed to callers.
type AnchorGateStatus string

const (
	AnchorGateStatusOK           AnchorGateStatus = "OK"
	AnchorGateStatusFrozenAnchor AnchorGateStatus = "FROZEN_ANCHOR"
)

// Thresholds controls when warning/freezing occurs.
type Thresholds struct {
	HMin  float64
	HWarn float64
}

// DefaultThresholds returns Wave 1 defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{
		HMin:  0.70,
		HWarn: 0.80,
	}
}

// ThresholdsFromEnv loads optional overrides:
// GT_ANCHOR_HEALTH_H_MIN and GT_ANCHOR_HEALTH_H_WARN.
func ThresholdsFromEnv() Thresholds {
	thresholds := DefaultThresholds()

	if raw := strings.TrimSpace(os.Getenv("GT_ANCHOR_HEALTH_H_MIN")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			thresholds.HMin = clamp01(v)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("GT_ANCHOR_HEALTH_H_WARN")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			thresholds.HWarn = clamp01(v)
		}
	}

	// Keep warning strictly above freeze threshold.
	if thresholds.HWarn <= thresholds.HMin {
		thresholds.HWarn = thresholds.HMin + 0.05
		if thresholds.HWarn > 1.0 {
			thresholds.HWarn = 1.0
		}
		if thresholds.HWarn <= thresholds.HMin {
			thresholds.HMin = 0.70
			thresholds.HWarn = 0.80
		}
	}

	return thresholds
}

// AnchorTerms are the independent anchor-health factors.
type AnchorTerms struct {
	PredictiveValidity  float64 `json:"predictive_validity"`
	ExternalConcordance float64 `json:"external_concordance"`
	CalibrationQuality  float64 `json:"calibration_quality"`
	Coverage            float64 `json:"coverage"`
}

// Health computes the multiplicative anchor-health score.
func (t AnchorTerms) Health() float64 {
	return clamp01(t.PredictiveValidity) *
		clamp01(t.ExternalConcordance) *
		clamp01(t.CalibrationQuality) *
		clamp01(t.Coverage)
}

// AnchorHealthSnapshot is produced by the independent anchor-health pipeline.
type AnchorHealthSnapshot struct {
	Version             int         `json:"version"`
	PipelineVersion     string      `json:"pipeline_version,omitempty"`
	ComputedAt          string      `json:"computed_at,omitempty"`
	WindowMinutes       int         `json:"window_minutes,omitempty"`
	SignatureAlg        string      `json:"signature_alg,omitempty"`
	Signer              string      `json:"signer,omitempty"`
	Signature           string      `json:"signature,omitempty"`
	AnchorHealth        float64     `json:"anchor_health,omitempty"`
	Terms               AnchorTerms `json:"terms"`
	DriftTrend          []float64   `json:"drift_trend,omitempty"`
	ContradictionDeltas []float64   `json:"contradiction_deltas,omitempty"`
	PredictiveDecay     []float64   `json:"predictive_decay,omitempty"`
	PointerHistory      []string    `json:"pointer_history,omitempty"`
}

// EffectiveHealth returns explicit anchor_health if present, otherwise terms product.
func (s *AnchorHealthSnapshot) EffectiveHealth() float64 {
	if s == nil {
		return 0
	}
	if s.AnchorHealth > 0 {
		return clamp01(s.AnchorHealth)
	}
	return s.Terms.Health()
}

// FreezeState captures why/when anchor freeze was triggered.
type FreezeState struct {
	TriggeredAt  string  `json:"triggered_at"`
	Reason       string  `json:"reason"`
	AnchorHealth float64 `json:"anchor_health"`
	ArtifactID   string  `json:"artifact_id,omitempty"`
	ArtifactHash string  `json:"artifact_hash,omitempty"`
}

// ControlPlaneState is the durable governance state.
type ControlPlaneState struct {
	Version             int          `json:"version"`
	SystemMode          SystemMode   `json:"system_mode"`
	MonitoringFrequency string       `json:"monitoring_frequency"`
	LastAnchorHealth    float64      `json:"last_anchor_health"`
	LastWarnAt          string       `json:"last_warn_at,omitempty"`
	BlockedPromotions   int64        `json:"blocked_promotions"`
	PromotionPointer    string       `json:"promotion_pointer,omitempty"`
	PointerHistory      []string     `json:"pointer_history,omitempty"`
	Freeze              *FreezeState `json:"freeze,omitempty"`
	UpdatedAt           string       `json:"updated_at"`
}

// AssertInput identifies the promotion lane currently requesting authorization.
type AssertInput struct {
	Lane             string
	PromotionPointer string
}

// AssertResult is returned by the choke-point check.
type AssertResult struct {
	Status            AnchorGateStatus `json:"status"`
	Mode              SystemMode       `json:"mode"`
	AnchorHealth      float64          `json:"anchor_health"`
	HMin              float64          `json:"h_min"`
	HWarn             float64          `json:"h_warn"`
	LatencyMs         int64            `json:"latency_ms,omitempty"`
	Reason            string           `json:"reason"`
	ArtifactID        string           `json:"artifact_id,omitempty"`
	BlockedPromotions int64            `json:"blocked_promotions"`
	PreFreezeWarning  bool             `json:"pre_freeze_warning,omitempty"`
}

// FreezeArtifact captures the required investigation payload.
type FreezeArtifact struct {
	Version             int         `json:"version"`
	ID                  string      `json:"id"`
	GeneratedAt         string      `json:"generated_at"`
	Reason              string      `json:"reason"`
	Lane                string      `json:"lane,omitempty"`
	PromotionPointer    string      `json:"promotion_pointer,omitempty"`
	AnchorHealth        float64     `json:"anchor_health"`
	Thresholds          Thresholds  `json:"thresholds"`
	TermBreakdown       AnchorTerms `json:"term_breakdown"`
	DriftTrend          []float64   `json:"drift_trend,omitempty"`
	ContradictionDeltas []float64   `json:"contradiction_deltas,omitempty"`
	PredictiveDecay     []float64   `json:"predictive_decay,omitempty"`
	PointerSnapshot     string      `json:"pointer_snapshot,omitempty"`
	PointerHistory      []string    `json:"pointer_history,omitempty"`
	SnapshotPath        string      `json:"snapshot_path"`
	StatePath           string      `json:"state_path"`
	PrevHash            string      `json:"prev_hash,omitempty"`
	Hash                string      `json:"hash"`
}

// Controller manages the anchor-health control plane.
type Controller struct {
	TownRoot   string
	Thresholds Thresholds
	Now        func() time.Time
}

// NewController constructs a governance controller for a town root.
func NewController(townRoot string, thresholds Thresholds) *Controller {
	if thresholds.HMin <= 0 || thresholds.HWarn <= 0 {
		thresholds = DefaultThresholds()
	}
	if thresholds.HWarn <= thresholds.HMin {
		thresholds.HWarn = thresholds.HMin + 0.05
		if thresholds.HWarn > 1.0 {
			thresholds.HWarn = 1.0
		}
	}
	return &Controller{
		TownRoot:   townRoot,
		Thresholds: thresholds,
		Now:        time.Now,
	}
}

// AssertAnchorHealth is the single pre-promotion choke point.
func (c *Controller) AssertAnchorHealth(input AssertInput) (*AssertResult, error) {
	if strings.TrimSpace(c.TownRoot) == "" {
		return &AssertResult{
			Status:       AnchorGateStatusOK,
			Mode:         SystemModeNormal,
			AnchorHealth: 1.0,
			HMin:         c.Thresholds.HMin,
			HWarn:        c.Thresholds.HWarn,
			Reason:       "town root unavailable; gate bypassed",
		}, nil
	}

	if err := os.MkdirAll(c.governanceDir(), 0755); err != nil {
		return nil, fmt.Errorf("creating governance dir: %w", err)
	}

	governanceMutex.Lock()
	defer governanceMutex.Unlock()

	unlock, err := lock.FlockAcquire(c.lockPath())
	if err != nil {
		return nil, fmt.Errorf("acquiring governance lock: %w", err)
	}
	defer unlock()

	state, err := c.loadStateUnsafe()
	if err != nil {
		return nil, err
	}

	snapshot, snapshotMissing, snapshotErr := c.loadSnapshotUnsafe()
	health := snapshot.EffectiveHealth()
	now := c.nowUTC()

	result := &AssertResult{
		Status:       AnchorGateStatusOK,
		Mode:         state.SystemMode,
		AnchorHealth: health,
		HMin:         c.Thresholds.HMin,
		HWarn:        c.Thresholds.HWarn,
	}

	// System mode blocks promotions before any score checks.
	if state.SystemMode != SystemModeNormal {
		state.LastAnchorHealth = health
		state.BlockedPromotions++
		state.UpdatedAt = now
		if saveErr := c.saveStateUnsafe(state); saveErr != nil {
			return nil, saveErr
		}

		reason := fmt.Sprintf("promotion blocked by system mode %s", state.SystemMode)
		result.Status = AnchorGateStatusFrozenAnchor
		result.Mode = state.SystemMode
		result.Reason = reason
		result.BlockedPromotions = state.BlockedPromotions
		if state.Freeze != nil {
			result.ArtifactID = state.Freeze.ArtifactID
		}
		c.logGateEvent(events.TypeAnchorHealthGate, "blocked_mode", input, result)
		if snapshotErr != nil {
			return result, fmt.Errorf("loading anchor health snapshot: %w", snapshotErr)
		}
		return result, nil
	}

	// Pre-freeze warning path. This is non-blocking and never partially gates.
	if health < c.Thresholds.HWarn {
		state.MonitoringFrequency = monitoringEscalated
		state.LastWarnAt = now
		result.PreFreezeWarning = true
		c.logGateEvent(events.TypeAnchorHealthWarn, "pre_freeze", input, result)
	} else {
		state.MonitoringFrequency = monitoringNormal
	}

	state.LastAnchorHealth = health
	state.UpdatedAt = now

	freezeReason := ""
	if snapshotErr != nil {
		if isSignatureValidationError(snapshotErr) {
			freezeReason = fmt.Sprintf("anchor health signature validation failed: %v", snapshotErr)
		} else {
			freezeReason = fmt.Sprintf("anchor health snapshot parse failure: %v", snapshotErr)
		}
	} else if snapshotMissing {
		freezeReason = "anchor health snapshot missing"
	} else if health < c.Thresholds.HMin {
		freezeReason = fmt.Sprintf("anchor health %.3f < H_min %.3f", health, c.Thresholds.HMin)
	}

	if freezeReason != "" {
		if isSignatureValidationError(snapshotErr) {
			result.Status = AnchorGateStatusFrozenAnchor
			result.Reason = freezeReason
			c.logGateEvent(events.TypeAnchorHealthGate, "signature_invalid", input, result)
		}

		// Freeze is committed first. Artifact generation is best-effort afterward.
		state.SystemMode = SystemModeAnchorFreeze
		state.BlockedPromotions++
		state.Freeze = &FreezeState{
			TriggeredAt:  now,
			Reason:       freezeReason,
			AnchorHealth: health,
		}

		if err := c.saveStateUnsafe(state); err != nil {
			return nil, err
		}

		artifact, artifactErr := c.writeFreezeArtifactUnsafe(state, snapshot, input, freezeReason)
		if artifactErr == nil {
			state.Freeze.ArtifactID = artifact.ID
			state.Freeze.ArtifactHash = artifact.Hash
			state.UpdatedAt = now
			if err := c.saveStateUnsafe(state); err != nil {
				artifactErr = fmt.Errorf("persisting artifact linkage: %w", err)
			}
		}

		result.Status = AnchorGateStatusFrozenAnchor
		result.Mode = SystemModeAnchorFreeze
		result.Reason = freezeReason
		result.BlockedPromotions = state.BlockedPromotions
		if state.Freeze != nil {
			result.ArtifactID = state.Freeze.ArtifactID
		}
		c.logFreezeEvent(input, result, freezeReason)

		if artifactErr != nil {
			return result, artifactErr
		}
		if snapshotErr != nil {
			return result, fmt.Errorf("loading anchor health snapshot: %w", snapshotErr)
		}
		return result, nil
	}

	if err := c.saveStateUnsafe(state); err != nil {
		return nil, err
	}

	result.Status = AnchorGateStatusOK
	result.Mode = SystemModeNormal
	result.Reason = "anchor health above freeze threshold"
	result.BlockedPromotions = state.BlockedPromotions
	c.logGateEvent(events.TypeAnchorHealthGate, "ok", input, result)
	return result, nil
}

// RecordPromotionPointer updates the active promotion pointer in NORMAL mode.
func (c *Controller) RecordPromotionPointer(pointer string) error {
	if strings.TrimSpace(c.TownRoot) == "" || strings.TrimSpace(pointer) == "" {
		return nil
	}
	if err := os.MkdirAll(c.governanceDir(), 0755); err != nil {
		return fmt.Errorf("creating governance dir: %w", err)
	}

	governanceMutex.Lock()
	defer governanceMutex.Unlock()

	unlock, err := lock.FlockAcquire(c.lockPath())
	if err != nil {
		return fmt.Errorf("acquiring governance lock: %w", err)
	}
	defer unlock()

	state, err := c.loadStateUnsafe()
	if err != nil {
		return err
	}
	if state.SystemMode != SystemModeNormal {
		return fmt.Errorf("system_mode=%s blocks pointer updates", state.SystemMode)
	}

	state.PromotionPointer = pointer
	state.PointerHistory = append(state.PointerHistory, pointer)
	if len(state.PointerHistory) > 64 {
		state.PointerHistory = append([]string{}, state.PointerHistory[len(state.PointerHistory)-64:]...)
	}
	state.UpdatedAt = c.nowUTC()

	return c.saveStateUnsafe(state)
}

// LoadState reads the control-plane state from disk.
func (c *Controller) LoadState() (*ControlPlaneState, error) {
	if strings.TrimSpace(c.TownRoot) == "" {
		return defaultState(), nil
	}
	governanceMutex.Lock()
	defer governanceMutex.Unlock()

	unlock, err := lock.FlockAcquire(c.lockPath())
	if err != nil {
		return nil, fmt.Errorf("acquiring governance lock: %w", err)
	}
	defer unlock()
	return c.loadStateUnsafe()
}

// Unfreeze requires both artifact linkage and an external attestation token.
func (c *Controller) Unfreeze(artifactID, attestation string) error {
	artifactID = strings.TrimSpace(artifactID)
	attestation = strings.TrimSpace(attestation)

	if strings.TrimSpace(c.TownRoot) == "" {
		return fmt.Errorf("town root is required")
	}
	if artifactID == "" {
		return fmt.Errorf("artifact ID is required")
	}
	if attestation == "" {
		return fmt.Errorf("attestation is required")
	}

	governanceMutex.Lock()
	defer governanceMutex.Unlock()

	unlock, err := lock.FlockAcquire(c.lockPath())
	if err != nil {
		return fmt.Errorf("acquiring governance lock: %w", err)
	}
	defer unlock()

	state, err := c.loadStateUnsafe()
	if err != nil {
		return err
	}
	if state.SystemMode != SystemModeAnchorFreeze {
		return fmt.Errorf("system mode is %s, not ANCHOR_FREEZE", state.SystemMode)
	}
	if state.Freeze == nil || strings.TrimSpace(state.Freeze.ArtifactID) == "" {
		return fmt.Errorf("freeze artifact linkage missing")
	}
	if state.Freeze.ArtifactID != artifactID {
		return fmt.Errorf("artifact mismatch: expected %s", state.Freeze.ArtifactID)
	}

	snapshot, snapshotMissing, snapshotErr := c.loadSnapshotUnsafe()
	if snapshotErr != nil {
		return fmt.Errorf("revalidating anchor health snapshot: %w", snapshotErr)
	}
	if snapshotMissing {
		return fmt.Errorf("anchor health snapshot missing during unfreeze")
	}
	health := snapshot.EffectiveHealth()
	if health < c.Thresholds.HMin {
		return fmt.Errorf("anchor health %.3f below H_min %.3f; remain frozen", health, c.Thresholds.HMin)
	}

	state.SystemMode = SystemModeNormal
	state.MonitoringFrequency = monitoringNormal
	state.Freeze = nil
	state.UpdatedAt = c.nowUTC()

	if err := c.saveStateUnsafe(state); err != nil {
		return err
	}

	payload := map[string]interface{}{
		"schema_version": governanceEventSchema,
		"artifact_id":    artifactID,
		"attestation":    attestation,
		"anchor_health":  health,
		"h_min":          c.Thresholds.HMin,
		"system_mode":    string(SystemModeNormal),
		"event_marker":   "Anchor Health Unfreeze Event",
	}
	_ = logAuditEventFn(events.TypeAnchorHealthUnfreeze, c.eventActor(), payload)
	return nil
}

func (c *Controller) governanceDir() string {
	return filepath.Join(c.TownRoot, "mayor", "governance")
}

func (c *Controller) statePath() string {
	return filepath.Join(c.governanceDir(), "system_mode.json")
}

func (c *Controller) snapshotPath() string {
	return filepath.Join(c.governanceDir(), "anchor_health.json")
}

func (c *Controller) lockPath() string {
	return filepath.Join(c.governanceDir(), ".anchor_health.lock")
}

func (c *Controller) artifactsDir() string {
	return filepath.Join(c.governanceDir(), "anchor_freeze_artifacts")
}

func (c *Controller) artifactLogPath() string {
	return filepath.Join(c.governanceDir(), "anchor_freeze_artifacts.jsonl")
}

func (c *Controller) loadStateUnsafe() (*ControlPlaneState, error) {
	path := c.statePath()
	data, err := os.ReadFile(path) //nolint:gosec // internal control-plane state path
	if err != nil {
		if os.IsNotExist(err) {
			return defaultState(), nil
		}
		return nil, fmt.Errorf("reading governance state: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return defaultState(), nil
	}

	var state ControlPlaneState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing governance state: %w", err)
	}
	if state.Version == 0 {
		state.Version = governanceStateVersion
	}
	if state.SystemMode == "" {
		state.SystemMode = SystemModeNormal
	}
	if state.MonitoringFrequency == "" {
		state.MonitoringFrequency = monitoringNormal
	}
	return &state, nil
}

func (c *Controller) saveStateUnsafe(state *ControlPlaneState) error {
	if state == nil {
		return nil
	}
	state.Version = governanceStateVersion
	if state.SystemMode == "" {
		state.SystemMode = SystemModeNormal
	}
	if state.MonitoringFrequency == "" {
		state.MonitoringFrequency = monitoringNormal
	}
	if state.UpdatedAt == "" {
		state.UpdatedAt = c.nowUTC()
	}
	return util.EnsureDirAndWriteJSON(c.statePath(), state)
}

func (c *Controller) loadSnapshotUnsafe() (*AnchorHealthSnapshot, bool, error) {
	data, err := os.ReadFile(c.snapshotPath()) //nolint:gosec // internal control-plane snapshot path
	if err != nil {
		if os.IsNotExist(err) {
			return &AnchorHealthSnapshot{}, true, nil
		}
		return &AnchorHealthSnapshot{}, true, fmt.Errorf("reading anchor health snapshot: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &AnchorHealthSnapshot{}, true, nil
	}

	var snapshot AnchorHealthSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return &AnchorHealthSnapshot{}, true, fmt.Errorf("parsing anchor health snapshot: %w", err)
	}
	if err := validateSnapshotSignature(&snapshot); err != nil {
		return &AnchorHealthSnapshot{}, true, fmt.Errorf("validating anchor health signature: %w", err)
	}
	return &snapshot, false, nil
}

func (c *Controller) writeFreezeArtifactUnsafe(state *ControlPlaneState, snapshot *AnchorHealthSnapshot, input AssertInput, reason string) (*FreezeArtifact, error) {
	if err := os.MkdirAll(c.artifactsDir(), 0755); err != nil {
		return nil, fmt.Errorf("creating artifacts dir: %w", err)
	}

	prevHash, err := readLastArtifactHash(c.artifactLogPath())
	if err != nil {
		return nil, fmt.Errorf("reading artifact hash chain: %w", err)
	}

	now := c.nowUTC()
	artifact := &FreezeArtifact{
		Version:             freezeArtifactVersion,
		GeneratedAt:         now,
		Reason:              reason,
		Lane:                strings.TrimSpace(input.Lane),
		PromotionPointer:    strings.TrimSpace(input.PromotionPointer),
		AnchorHealth:        state.LastAnchorHealth,
		Thresholds:          c.Thresholds,
		TermBreakdown:       snapshot.Terms,
		DriftTrend:          append([]float64{}, snapshot.DriftTrend...),
		ContradictionDeltas: append([]float64{}, snapshot.ContradictionDeltas...),
		PredictiveDecay:     append([]float64{}, snapshot.PredictiveDecay...),
		PointerSnapshot:     state.PromotionPointer,
		PointerHistory:      append([]string{}, state.PointerHistory...),
		SnapshotPath:        c.snapshotPath(),
		StatePath:           c.statePath(),
		PrevHash:            prevHash,
	}

	seed := fmt.Sprintf("%s|%s|%s|%0.6f", now, reason, artifact.PromotionPointer, artifact.AnchorHealth)
	seedSum := sha256.Sum256([]byte(seed))

	artifactPath := ""
	var file *os.File
	for attempt := 0; attempt < 6; attempt++ {
		artifact.ID = newFreezeArtifactID(c.Now().UTC(), attempt, seedSum[:])

		hash, hashErr := computeArtifactHash(artifact)
		if hashErr != nil {
			return nil, hashErr
		}
		artifact.Hash = hash

		artifactPath = filepath.Join(c.artifactsDir(), artifact.ID+".json")
		file, err = os.OpenFile(artifactPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err == nil {
			break
		}
		if os.IsExist(err) {
			continue
		}
		return nil, fmt.Errorf("creating freeze artifact: %w", err)
	}
	if file == nil {
		return nil, fmt.Errorf("creating freeze artifact: exhausted ID retries")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		file.Close()
		return nil, fmt.Errorf("writing freeze artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("closing freeze artifact: %w", err)
	}

	logFile, err := os.OpenFile(c.artifactLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // append-only operational audit log
	if err != nil {
		return nil, fmt.Errorf("opening artifact log: %w", err)
	}
	line, err := json.Marshal(artifact)
	if err != nil {
		logFile.Close()
		return nil, fmt.Errorf("marshaling artifact log line: %w", err)
	}
	if _, err := logFile.Write(append(line, '\n')); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("appending artifact log: %w", err)
	}
	if err := logFile.Close(); err != nil {
		return nil, fmt.Errorf("closing artifact log: %w", err)
	}

	return artifact, nil
}

func (c *Controller) logGateEvent(eventType, phase string, input AssertInput, result *AssertResult) {
	payload := map[string]interface{}{
		"schema_version":    governanceEventSchema,
		"phase":             phase,
		"lane":              strings.TrimSpace(input.Lane),
		"promotion_pointer": strings.TrimSpace(input.PromotionPointer),
		"status":            string(result.Status),
		"system_mode":       string(result.Mode),
		"anchor_health":     result.AnchorHealth,
		"h_min":             result.HMin,
		"h_warn":            result.HWarn,
		"reason":            result.Reason,
		"blocked_count":     result.BlockedPromotions,
	}
	if result.ArtifactID != "" {
		payload["artifact_id"] = result.ArtifactID
	}
	if result.LatencyMs > 0 {
		payload["latency_ms"] = result.LatencyMs
	}
	_ = logAuditEventFn(eventType, c.eventActor(), payload)
}

func (c *Controller) logFreezeEvent(input AssertInput, result *AssertResult, reason string) {
	payload := map[string]interface{}{
		"schema_version":       governanceEventSchema,
		"phase":                "freeze",
		"lane":                 strings.TrimSpace(input.Lane),
		"promotion_pointer":    strings.TrimSpace(input.PromotionPointer),
		"status":               string(result.Status),
		"system_mode":          string(result.Mode),
		"anchor_health":        result.AnchorHealth,
		"h_min":                result.HMin,
		"h_warn":               result.HWarn,
		"reason":               reason,
		"blocked_count":        result.BlockedPromotions,
		"dashboard_annotation": "Anchor Health Freeze Event",
	}
	if result.ArtifactID != "" {
		payload["artifact_id"] = result.ArtifactID
	}
	if result.LatencyMs > 0 {
		payload["latency_ms"] = result.LatencyMs
	}
	_ = logFeedEventFn(events.TypeAnchorHealthFreeze, c.eventActor(), payload)
}

func (c *Controller) eventActor() string {
	role := strings.TrimSpace(os.Getenv("GT_ROLE"))
	if role == "" {
		return "mayor"
	}
	return role
}

func (c *Controller) nowUTC() string {
	return c.Now().UTC().Format(time.RFC3339)
}

func defaultState() *ControlPlaneState {
	return &ControlPlaneState{
		Version:             governanceStateVersion,
		SystemMode:          SystemModeNormal,
		MonitoringFrequency: monitoringNormal,
		PointerHistory:      []string{},
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func computeArtifactHash(artifact *FreezeArtifact) (string, error) {
	clone := *artifact
	clone.Hash = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("marshaling artifact for hash: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func readLastArtifactHash(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // internal append-only artifact log
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	var lastHash string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry struct {
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Hash != "" {
			lastHash = entry.Hash
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return lastHash, nil
}

func newFreezeArtifactID(now time.Time, attempt int, seed []byte) string {
	var entropy [4]byte
	if _, err := crand.Read(entropy[:]); err != nil {
		sum := sha256.Sum256(append(seed, byte(attempt)))
		copy(entropy[:], sum[:4])
	}
	return fmt.Sprintf("af-%d-%s", now.UnixNano(), hex.EncodeToString(entropy[:]))
}

func validateSnapshotSignature(snapshot *AnchorHealthSnapshot) error {
	requireSignature := requiresAnchorSnapshotSignature()
	pubKeyRaw := strings.TrimSpace(os.Getenv("GT_ANCHOR_HEALTH_PUBKEY"))
	if pubKeyRaw == "" {
		if requireSignature {
			return fmt.Errorf("GT_ANCHOR_HEALTH_PUBKEY is required in production")
		}
		return nil
	}

	if snapshot == nil {
		return fmt.Errorf("snapshot is nil")
	}
	if strings.TrimSpace(snapshot.Signature) == "" {
		return fmt.Errorf("missing signature")
	}

	alg := strings.TrimSpace(strings.ToLower(snapshot.SignatureAlg))
	if alg != "" && alg != "ed25519" {
		return fmt.Errorf("unsupported signature_alg: %s", snapshot.SignatureAlg)
	}

	pub, err := decodePublicKey(pubKeyRaw)
	if err != nil {
		return fmt.Errorf("decoding public key: %w", err)
	}
	sig, err := decodeBase64(snapshot.Signature)
	if err != nil {
		return fmt.Errorf("decoding signature: %w", err)
	}

	clone := *snapshot
	clone.Signature = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return fmt.Errorf("marshaling signed payload: %w", err)
	}
	if !ed25519.Verify(pub, payload, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func requiresAnchorSnapshotSignature() bool {
	if raw := strings.TrimSpace(os.Getenv("GT_ANCHOR_HEALTH_REQUIRE_SIGNATURE")); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GT_GOVERNANCE_ENV"))) {
	case "prod", "production":
		return true
	}
	return false
}

func isSignatureValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "validating anchor health signature") ||
		strings.Contains(msg, "signature validation failed") ||
		strings.Contains(msg, "missing signature") ||
		strings.Contains(msg, "public key")
}

func decodePublicKey(raw string) (ed25519.PublicKey, error) {
	decoded, err := decodeBase64(raw)
	if err != nil {
		decoded, err = hex.DecodeString(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("expected base64 or hex")
		}
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key length %d != %d", len(decoded), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func decodeBase64(raw string) ([]byte, error) {
	candidates := []string{
		strings.TrimSpace(raw),
		strings.TrimRight(strings.TrimSpace(raw), "="),
	}
	var firstErr error
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if out, err := base64.StdEncoding.DecodeString(c); err == nil {
			return out, nil
		} else if firstErr == nil {
			firstErr = err
		}
		if out, err := base64.RawStdEncoding.DecodeString(c); err == nil {
			return out, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("empty input")
}
