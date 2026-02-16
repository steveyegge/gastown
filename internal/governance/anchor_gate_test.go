package governance

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAssertAnchorHealth_PreFreezeWarningDoesNotBlock(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.00,
			ExternalConcordance: 0.90,
			CalibrationQuality:  0.90,
			Coverage:            0.95,
		},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-1",
	})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusOK {
		t.Fatalf("Status = %s, want %s", result.Status, AnchorGateStatusOK)
	}
	if !result.PreFreezeWarning {
		t.Fatal("expected pre-freeze warning flag")
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.MonitoringFrequency != monitoringEscalated {
		t.Fatalf("MonitoringFrequency = %q, want %q", state.MonitoringFrequency, monitoringEscalated)
	}
}

func TestAssertAnchorHealth_FreezesAndCreatesArtifact(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.95,
			ExternalConcordance: 0.95,
			CalibrationQuality:  0.95,
			Coverage:            0.50, // 0.428 < H_min
		},
		DriftTrend:          []float64{0.82, 0.78, 0.74},
		ContradictionDeltas: []float64{0.02, 0.04},
		PredictiveDecay:     []float64{0.91, 0.88, 0.83},
		PointerHistory:      []string{"w-prev"},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-2",
	})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("Status = %s, want %s", result.Status, AnchorGateStatusFrozenAnchor)
	}
	if result.ArtifactID == "" {
		t.Fatal("expected artifact ID on freeze")
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.SystemMode != SystemModeAnchorFreeze {
		t.Fatalf("SystemMode = %s, want %s", state.SystemMode, SystemModeAnchorFreeze)
	}
	if state.Freeze == nil || state.Freeze.ArtifactID == "" {
		t.Fatal("freeze state missing artifact linkage")
	}

	artifactPath := filepath.Join(townRoot, "mayor", "governance", "anchor_freeze_artifacts", state.Freeze.ArtifactID+".json")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("artifact file missing: %v", err)
	}
}

func TestAssertAnchorHealth_FrozenModeBlocksEvenWhenHealthRecovers(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.9,
			ExternalConcordance: 0.9,
			CalibrationQuality:  0.9,
			Coverage:            0.4,
		},
	})
	first, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-low"})
	if err != nil {
		t.Fatalf("first AssertAnchorHealth() error = %v", err)
	}
	if first.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("first status = %s, want %s", first.Status, AnchorGateStatusFrozenAnchor)
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.0,
			ExternalConcordance: 1.0,
			CalibrationQuality:  1.0,
			Coverage:            1.0,
		},
	})
	second, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-high"})
	if err != nil {
		t.Fatalf("second AssertAnchorHealth() error = %v", err)
	}
	if second.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("second status = %s, want %s", second.Status, AnchorGateStatusFrozenAnchor)
	}
	if second.Mode != SystemModeAnchorFreeze {
		t.Fatalf("second mode = %s, want %s", second.Mode, SystemModeAnchorFreeze)
	}
}

func TestAssertAnchorHealth_ArtifactFailureStillHoldsFreeze(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	governanceDir := filepath.Join(townRoot, "mayor", "governance")
	if err := os.MkdirAll(governanceDir, 0755); err != nil {
		t.Fatalf("mkdir governance dir: %v", err)
	}
	// Force artifact creation failure by making artifacts dir path a file.
	artifactDirPath := filepath.Join(governanceDir, "anchor_freeze_artifacts")
	if err := os.WriteFile(artifactDirPath, []byte("not-a-directory"), 0644); err != nil {
		t.Fatalf("write fake artifact dir file: %v", err)
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.5,
			ExternalConcordance: 0.5,
			CalibrationQuality:  0.5,
			Coverage:            0.5,
		},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-3",
	})
	if err == nil {
		t.Fatal("expected artifact generation error, got nil")
	}
	if result == nil || result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %v, want %s", result, AnchorGateStatusFrozenAnchor)
	}

	state, stateErr := controller.LoadState()
	if stateErr != nil {
		t.Fatalf("LoadState() error = %v", stateErr)
	}
	if state.SystemMode != SystemModeAnchorFreeze {
		t.Fatalf("SystemMode = %s, want %s", state.SystemMode, SystemModeAnchorFreeze)
	}
}

func TestUnfreezeRequiresArtifactAndAttestation(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.8,
			ExternalConcordance: 0.8,
			CalibrationQuality:  0.8,
			Coverage:            0.5,
		},
	})
	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-4",
	})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %s, want %s", result.Status, AnchorGateStatusFrozenAnchor)
	}

	if err := controller.Unfreeze(result.ArtifactID, ""); err == nil {
		t.Fatal("expected attestation-required error")
	}
	if err := controller.Unfreeze("wrong-artifact", "signed-attestation"); err == nil {
		t.Fatal("expected artifact mismatch error")
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.0,
			ExternalConcordance: 1.0,
			CalibrationQuality:  1.0,
			Coverage:            1.0,
		},
	})
	if err := controller.Unfreeze(result.ArtifactID, "signed-attestation"); err != nil {
		t.Fatalf("Unfreeze() error = %v", err)
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.SystemMode != SystemModeNormal {
		t.Fatalf("SystemMode = %s, want %s", state.SystemMode, SystemModeNormal)
	}
}

func TestUnfreezeFailsIfAnchorStillBelowHMin(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.7,
			ExternalConcordance: 0.7,
			CalibrationQuality:  0.7,
			Coverage:            0.7,
		},
	})
	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-still-low",
	})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %s, want %s", result.Status, AnchorGateStatusFrozenAnchor)
	}

	if err := controller.Unfreeze(result.ArtifactID, "signed-attestation"); err == nil {
		t.Fatal("expected unfreeze to fail while anchor health is still below H_min")
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.SystemMode != SystemModeAnchorFreeze {
		t.Fatalf("SystemMode = %s, want %s", state.SystemMode, SystemModeAnchorFreeze)
	}
}

func TestRecordPromotionPointerBlockedDuringFreeze(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})
	controller.Now = func() time.Time {
		return time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})
	if _, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-5"}); err != nil {
		t.Fatalf("AssertAnchorHealth() healthy error = %v", err)
	}
	if err := controller.RecordPromotionPointer("w-5"); err != nil {
		t.Fatalf("RecordPromotionPointer() error = %v", err)
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.4,
			ExternalConcordance: 0.4,
			CalibrationQuality:  0.4,
			Coverage:            0.4,
		},
	})
	if _, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-6"}); err != nil {
		t.Fatalf("AssertAnchorHealth() freeze error = %v", err)
	}
	if err := controller.RecordPromotionPointer("w-6"); err == nil {
		t.Fatal("expected pointer update to fail while frozen")
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.PromotionPointer != "w-5" {
		t.Fatalf("PromotionPointer = %q, want %q", state.PromotionPointer, "w-5")
	}
}

func TestAssertAnchorHealth_ProductionRequiresSignatureKey(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	t.Setenv("GT_GOVERNANCE_ENV", "production")
	t.Setenv("GT_ANCHOR_HEALTH_PUBKEY", "")

	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})
	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.0,
			ExternalConcordance: 1.0,
			CalibrationQuality:  1.0,
			Coverage:            1.0,
		},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-prod-nosigkey",
	})
	if err == nil {
		t.Fatal("expected signature-key validation error")
	}
	if result == nil || result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %v, want %s", result, AnchorGateStatusFrozenAnchor)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pubkey") {
		t.Fatalf("expected pubkey requirement error, got: %v", err)
	}
}

func TestAssertAnchorHealth_ProductionRequiresSignedSnapshot(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	t.Setenv("GT_GOVERNANCE_ENV", "production")

	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_ANCHOR_HEALTH_PUBKEY", base64.StdEncoding.EncodeToString(pub))

	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})
	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.0,
			ExternalConcordance: 1.0,
			CalibrationQuality:  1.0,
			Coverage:            1.0,
		},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-prod-unsigned",
	})
	if err == nil {
		t.Fatal("expected missing signature error")
	}
	if result == nil || result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %v, want %s", result, AnchorGateStatusFrozenAnchor)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "missing signature") {
		t.Fatalf("expected missing signature error, got: %v", err)
	}
}

func TestAssertAnchorHealth_ValidSignaturePasses(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_ANCHOR_HEALTH_PUBKEY", base64.StdEncoding.EncodeToString(pub))

	snapshot := &AnchorHealthSnapshot{
		Version:      1,
		SignatureAlg: "ed25519",
		Signer:       "pipeline/test",
		Terms: AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	}
	signSnapshot(t, snapshot, priv)
	writeSnapshot(t, townRoot, snapshot)

	result, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-signed"})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusOK {
		t.Fatalf("Status = %s, want %s", result.Status, AnchorGateStatusOK)
	}
}

func TestAssertAnchorHealth_InvalidSignatureFreezes(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_ANCHOR_HEALTH_PUBKEY", base64.StdEncoding.EncodeToString(pub))

	snapshot := &AnchorHealthSnapshot{
		Version:      1,
		SignatureAlg: "ed25519",
		Signer:       "pipeline/test",
		Terms: AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	}
	signSnapshot(t, snapshot, priv)
	// Mutate post-signature to force verification failure.
	snapshot.Terms.Coverage = 0.9
	writeSnapshot(t, townRoot, snapshot)

	result, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-bad-signature"})
	if err == nil {
		t.Fatal("expected signature validation error")
	}
	if result == nil || result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("Status = %v, want %s", result, AnchorGateStatusFrozenAnchor)
	}
}

func TestAssertAnchorHealth_EventFailureDoesNotBypassFreeze(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	prevAudit := logAuditEventFn
	prevFeed := logFeedEventFn
	t.Cleanup(func() {
		logAuditEventFn = prevAudit
		logFeedEventFn = prevFeed
	})
	logAuditEventFn = func(eventType, actor string, payload map[string]interface{}) error {
		return os.ErrPermission
	}
	logFeedEventFn = func(eventType, actor string, payload map[string]interface{}) error {
		return os.ErrPermission
	}

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.4,
			ExternalConcordance: 0.4,
			CalibrationQuality:  0.4,
			Coverage:            0.4,
		},
	})

	result, err := controller.AssertAnchorHealth(AssertInput{Lane: "wisp_compaction", PromotionPointer: "w-events"})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("Status = %s, want %s", result.Status, AnchorGateStatusFrozenAnchor)
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.SystemMode != SystemModeAnchorFreeze {
		t.Fatalf("SystemMode = %s, want %s", state.SystemMode, SystemModeAnchorFreeze)
	}
}

func TestAssertAnchorHealth_LatencyBudget(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})
	t.Setenv("GT_ANCHOR_HEALTH_PUBKEY", "")

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})

	const iterations = 25
	var maxLatency time.Duration
	for i := 0; i < iterations; i++ {
		start := time.Now()
		result, err := controller.AssertAnchorHealth(AssertInput{
			Lane:             "wisp_compaction",
			PromotionPointer: "w-latency",
		})
		if err != nil {
			t.Fatalf("AssertAnchorHealth() error = %v", err)
		}
		if result.Status != AnchorGateStatusOK {
			t.Fatalf("Status = %s, want %s", result.Status, AnchorGateStatusOK)
		}
		latency := time.Since(start)
		if latency > maxLatency {
			maxLatency = latency
		}
	}

	if maxLatency > 500*time.Millisecond {
		t.Fatalf("max gate latency = %v, exceeds 500ms budget", maxLatency)
	}
	t.Logf("max gate latency across %d assertions: %v", iterations, maxLatency)
}

func TestSimulatedFreezeDrill(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	// Phase 1: healthy anchors allow promotion gate.
	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})
	okResult, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-before-freeze",
	})
	if err != nil {
		t.Fatalf("phase1 AssertAnchorHealth() error = %v", err)
	}
	if okResult.Status != AnchorGateStatusOK {
		t.Fatalf("phase1 status = %s, want %s", okResult.Status, AnchorGateStatusOK)
	}
	if err := controller.RecordPromotionPointer("w-before-freeze"); err != nil {
		t.Fatalf("phase1 RecordPromotionPointer() error = %v", err)
	}

	// Phase 2: synthetic drift triggers freeze.
	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.55,
			ExternalConcordance: 0.60,
			CalibrationQuality:  0.65,
			Coverage:            0.50,
		},
		DriftTrend:          []float64{0.85, 0.80, 0.75, 0.68},
		ContradictionDeltas: []float64{0.01, 0.03, 0.05},
	})
	freezeResult, err := controller.AssertAnchorHealth(AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-after-freeze",
	})
	if err != nil {
		t.Fatalf("phase2 AssertAnchorHealth() error = %v", err)
	}
	if freezeResult.Status != AnchorGateStatusFrozenAnchor {
		t.Fatalf("phase2 status = %s, want %s", freezeResult.Status, AnchorGateStatusFrozenAnchor)
	}
	if freezeResult.ArtifactID == "" {
		t.Fatal("phase2 expected artifact ID")
	}

	// Promotion pointer must remain stable during freeze.
	if err := controller.RecordPromotionPointer("w-after-freeze"); err == nil {
		t.Fatal("phase2 expected pointer update to fail while frozen")
	}
	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("phase2 LoadState() error = %v", err)
	}
	if state.PromotionPointer != "w-before-freeze" {
		t.Fatalf("phase2 pointer moved during freeze: got %q", state.PromotionPointer)
	}

	// Phase 3: controlled unfreeze with artifact + attestation.
	if err := controller.Unfreeze("wrong-artifact", "signed-attestation"); err == nil {
		t.Fatal("phase3 expected artifact mismatch error")
	}
	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  1.0,
			ExternalConcordance: 1.0,
			CalibrationQuality:  1.0,
			Coverage:            1.0,
		},
	})
	if err := controller.Unfreeze(freezeResult.ArtifactID, "signed-attestation"); err != nil {
		t.Fatalf("phase3 Unfreeze() error = %v", err)
	}
	unfrozen, err := controller.LoadState()
	if err != nil {
		t.Fatalf("phase3 LoadState() error = %v", err)
	}
	if unfrozen.SystemMode != SystemModeNormal {
		t.Fatalf("phase3 SystemMode = %s, want %s", unfrozen.SystemMode, SystemModeNormal)
	}
}

func TestAssertAnchorHealth_ConcurrentFreezeNoBypass(t *testing.T) {
	townRoot := t.TempDir()
	t.Chdir(townRoot)
	controller := NewController(townRoot, Thresholds{HMin: 0.70, HWarn: 0.80})

	writeSnapshot(t, townRoot, &AnchorHealthSnapshot{
		Version: 1,
		Terms: AnchorTerms{
			PredictiveValidity:  0.5,
			ExternalConcordance: 0.5,
			CalibrationQuality:  0.5,
			Coverage:            0.5,
		},
	})

	const workers = 12
	results := make([]*AssertResult, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = controller.AssertAnchorHealth(AssertInput{
				Lane:             "wisp_compaction",
				PromotionPointer: "w-concurrent",
			})
		}(i)
	}
	wg.Wait()

	var artifactID string
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d error = %v", i, errs[i])
		}
		if results[i] == nil {
			t.Fatalf("worker %d result is nil", i)
		}
		if results[i].Status != AnchorGateStatusFrozenAnchor {
			t.Fatalf("worker %d status = %s, want %s", i, results[i].Status, AnchorGateStatusFrozenAnchor)
		}
		if results[i].ArtifactID == "" {
			t.Fatalf("worker %d missing artifact ID", i)
		}
		if artifactID == "" {
			artifactID = results[i].ArtifactID
		}
		if results[i].ArtifactID != artifactID {
			t.Fatalf("worker %d artifact mismatch: got %s want %s", i, results[i].ArtifactID, artifactID)
		}
	}
}

func writeSnapshot(t *testing.T, townRoot string, snapshot *AnchorHealthSnapshot) {
	t.Helper()
	snapshotPath := filepath.Join(townRoot, "mayor", "governance", "anchor_health.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0755); err != nil {
		t.Fatalf("mkdir snapshot dir: %v", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(snapshotPath, data, 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

func signSnapshot(t *testing.T, snapshot *AnchorHealthSnapshot, priv ed25519.PrivateKey) {
	t.Helper()
	clone := *snapshot
	clone.Signature = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal signed payload: %v", err)
	}
	sig := ed25519.Sign(priv, payload)
	snapshot.Signature = strings.TrimRight(base64.StdEncoding.EncodeToString(sig), "=")
}
