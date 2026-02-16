package cmd

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	gov "github.com/steveyegge/gastown/internal/governance"
)

func TestCLIUnfreezeFailsIfNotFrozen(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	setGovernanceUnfreezeFlags(t, "deadbeef", filepath.Join(townRoot, "attestation.json"))

	err := runGovernanceUnfreeze(nil, nil)
	if err == nil {
		t.Fatal("expected unfreeze to fail when system is not frozen")
	}
	if !strings.Contains(err.Error(), "not ANCHOR_FREEZE") {
		t.Fatalf("expected not-frozen error, got: %v", err)
	}
}

func TestCLIUnfreezeFailsWithWrongArtifact(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	_, state := freezeGovernanceForCLI(t, townRoot)
	writeAnchorSnapshotForCLI(t, townRoot, &gov.AnchorHealthSnapshot{
		Version: 1,
		Terms: gov.AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})

	setGovernanceUnfreezeFlags(t, "wrong-hash", filepath.Join(townRoot, "attestation.json"))
	err := runGovernanceUnfreeze(nil, nil)
	if err == nil {
		t.Fatal("expected unfreeze to fail with artifact mismatch")
	}
	if !strings.Contains(err.Error(), "artifact hash mismatch") {
		t.Fatalf("expected artifact mismatch error, got: %v", err)
	}

	controller := gov.NewController(townRoot, gov.ThresholdsFromEnv())
	current, loadErr := controller.LoadState()
	if loadErr != nil {
		t.Fatalf("LoadState() error = %v", loadErr)
	}
	if current.SystemMode != gov.SystemModeAnchorFreeze {
		t.Fatalf("SystemMode = %s, want %s", current.SystemMode, gov.SystemModeAnchorFreeze)
	}
	if current.Freeze == nil || current.Freeze.ArtifactHash != state.Freeze.ArtifactHash {
		t.Fatal("freeze linkage changed after failed unfreeze")
	}
}

func TestCLIUnfreezeFailsWithInvalidSignature(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	_, state := freezeGovernanceForCLI(t, townRoot)
	writeAnchorSnapshotForCLI(t, townRoot, &gov.AnchorHealthSnapshot{
		Version: 1,
		Terms: gov.AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_GOVERNANCE_ATTESTATION_PUBKEYS", "safety="+base64.StdEncoding.EncodeToString(pub))
	t.Setenv("GT_GOVERNANCE_DOMAIN", "runtime")

	att := governanceAttestation{
		Version:      1,
		ArtifactHash: state.Freeze.ArtifactHash,
		SystemMode:   string(gov.SystemModeAnchorFreeze),
		Domain:       "safety",
		Signer:       "safety/reviewer-1",
		IssuedAt:     time.Now().UTC().Format(time.RFC3339),
		SignatureAlg: "ed25519",
	}
	signGovernanceAttestation(t, &att, priv)
	att.Signer = "safety/tampered"

	attestationPath := filepath.Join(townRoot, "attestation-invalid.json")
	writeGovernanceAttestation(t, attestationPath, &att)
	setGovernanceUnfreezeFlags(t, state.Freeze.ArtifactHash, attestationPath)

	err = runGovernanceUnfreeze(nil, nil)
	if err == nil {
		t.Fatal("expected invalid signature error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "signature") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}

func TestCLIUnfreezeFailsIfAnchorStillBelowHMin(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	_, state := freezeGovernanceForCLI(t, townRoot)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_GOVERNANCE_ATTESTATION_PUBKEYS", "safety="+base64.StdEncoding.EncodeToString(pub))
	t.Setenv("GT_GOVERNANCE_DOMAIN", "runtime")

	attestationPath := filepath.Join(townRoot, "attestation-low-health.json")
	writeSignedGovernanceAttestation(t, attestationPath, state.Freeze.ArtifactHash, "safety", "safety/reviewer-1", priv)
	setGovernanceUnfreezeFlags(t, state.Freeze.ArtifactHash, attestationPath)

	err = runGovernanceUnfreeze(nil, nil)
	if err == nil {
		t.Fatal("expected unfreeze to fail while anchor health is below H_min")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "below h_min") {
		t.Fatalf("expected H_min error, got: %v", err)
	}
}

func TestCLIUnfreezeSucceedsWithValidArtifactAndAttestation(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	controller, state := freezeGovernanceForCLI(t, townRoot)
	writeAnchorSnapshotForCLI(t, townRoot, &gov.AnchorHealthSnapshot{
		Version: 1,
		Terms: gov.AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_GOVERNANCE_ATTESTATION_PUBKEYS", "safety="+base64.StdEncoding.EncodeToString(pub))
	t.Setenv("GT_GOVERNANCE_DOMAIN", "runtime")

	attestationPath := filepath.Join(townRoot, "attestation-valid.json")
	writeSignedGovernanceAttestation(t, attestationPath, state.Freeze.ArtifactHash, "safety", "safety/reviewer-1", priv)
	setGovernanceUnfreezeFlags(t, state.Freeze.ArtifactHash, attestationPath)

	if err := runGovernanceUnfreeze(nil, nil); err != nil {
		t.Fatalf("runGovernanceUnfreeze() error = %v", err)
	}

	current, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if current.SystemMode != gov.SystemModeNormal {
		t.Fatalf("SystemMode = %s, want %s", current.SystemMode, gov.SystemModeNormal)
	}
}

func TestCLIUnfreezeEmitsGovernanceEvent(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	_, state := freezeGovernanceForCLI(t, townRoot)
	writeAnchorSnapshotForCLI(t, townRoot, &gov.AnchorHealthSnapshot{
		Version: 1,
		Terms: gov.AnchorTerms{
			PredictiveValidity:  1,
			ExternalConcordance: 1,
			CalibrationQuality:  1,
			Coverage:            1,
		},
	})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	t.Setenv("GT_GOVERNANCE_ATTESTATION_PUBKEYS", "safety="+base64.StdEncoding.EncodeToString(pub))
	t.Setenv("GT_GOVERNANCE_DOMAIN", "runtime")

	attestationPath := filepath.Join(townRoot, "attestation-event.json")
	writeSignedGovernanceAttestation(t, attestationPath, state.Freeze.ArtifactHash, "safety", "safety/reviewer-1", priv)
	setGovernanceUnfreezeFlags(t, state.Freeze.ArtifactHash, attestationPath)

	if err := runGovernanceUnfreeze(nil, nil); err != nil {
		t.Fatalf("runGovernanceUnfreeze() error = %v", err)
	}

	eventsPath := filepath.Join(townRoot, events.EventsFile)
	data, err := os.ReadFile(eventsPath) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", eventsPath, err)
	}

	found := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event events.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("unmarshal event line: %v", err)
		}
		if event.Type == events.TypeAnchorHealthUnfreeze {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s event in %s", events.TypeAnchorHealthUnfreeze, eventsPath)
	}
}

func TestCLIUnfreezeCannotBeCalledInBreakGlassWithoutArtifact(t *testing.T) {
	townRoot := setupGovernanceCLITown(t)
	t.Chdir(townRoot)

	statePath := filepath.Join(townRoot, "mayor", "governance", "system_mode.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	state := gov.ControlPlaneState{
		Version:             1,
		SystemMode:          gov.SystemModeBreakGlass,
		MonitoringFrequency: "NORMAL",
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(statePath, payload, 0644); err != nil { //nolint:gosec // test fixture state path
		t.Fatalf("WriteFile() error = %v", err)
	}

	setGovernanceUnfreezeFlags(t, "", filepath.Join(townRoot, "attestation.json"))

	err = runGovernanceUnfreeze(nil, nil)
	if err == nil {
		t.Fatal("expected missing artifact error")
	}
	if !strings.Contains(err.Error(), "--artifact is required") {
		t.Fatalf("expected required artifact error, got: %v", err)
	}
}

func setupGovernanceCLITown(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	townConfig := &config.TownConfig{
		Type:       "town",
		Version:    config.CurrentTownVersion,
		Name:       "test-town",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC),
	}
	if err := config.SaveTownConfig(filepath.Join(mayorDir, "town.json"), townConfig); err != nil {
		t.Fatalf("save town config: %v", err)
	}

	return townRoot
}

func freezeGovernanceForCLI(t *testing.T, townRoot string) (*gov.Controller, *gov.ControlPlaneState) {
	t.Helper()

	controller := gov.NewController(townRoot, gov.Thresholds{HMin: 0.70, HWarn: 0.80})
	writeAnchorSnapshotForCLI(t, townRoot, &gov.AnchorHealthSnapshot{
		Version: 1,
		Terms: gov.AnchorTerms{
			PredictiveValidity:  0.5,
			ExternalConcordance: 0.5,
			CalibrationQuality:  0.5,
			Coverage:            0.5,
		},
	})

	result, err := controller.AssertAnchorHealth(gov.AssertInput{
		Lane:             "wisp_compaction",
		PromotionPointer: "w-freeze",
	})
	if err != nil {
		t.Fatalf("AssertAnchorHealth() error = %v", err)
	}
	if result.Status != gov.AnchorGateStatusFrozenAnchor {
		t.Fatalf("status = %s, want %s", result.Status, gov.AnchorGateStatusFrozenAnchor)
	}

	state, err := controller.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Freeze == nil || state.Freeze.ArtifactHash == "" || state.Freeze.ArtifactID == "" {
		t.Fatal("expected freeze artifact linkage")
	}
	return controller, state
}

func writeAnchorSnapshotForCLI(t *testing.T, townRoot string, snapshot *gov.AnchorHealthSnapshot) {
	t.Helper()
	path := filepath.Join(townRoot, "mayor", "governance", "anchor_health.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir governance dir: %v", err)
	}
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, payload, 0644); err != nil { //nolint:gosec // fixture path
		t.Fatalf("write snapshot: %v", err)
	}
}

func setGovernanceUnfreezeFlags(t *testing.T, artifactHash, attestationPath string) {
	t.Helper()
	prevArtifact := governanceUnfreezeArtifactPath
	prevAttestation := governanceUnfreezeAttestation
	prevNow := governanceNow
	t.Cleanup(func() {
		governanceUnfreezeArtifactPath = prevArtifact
		governanceUnfreezeAttestation = prevAttestation
		governanceNow = prevNow
	})

	governanceUnfreezeArtifactPath = artifactHash
	governanceUnfreezeAttestation = attestationPath
	governanceNow = time.Now
}

func writeSignedGovernanceAttestation(t *testing.T, path, artifactHash, domain, signer string, priv ed25519.PrivateKey) {
	t.Helper()
	att := governanceAttestation{
		Version:      1,
		ArtifactHash: artifactHash,
		SystemMode:   string(gov.SystemModeAnchorFreeze),
		Domain:       domain,
		Signer:       signer,
		IssuedAt:     time.Now().UTC().Format(time.RFC3339),
		SignatureAlg: "ed25519",
	}
	signGovernanceAttestation(t, &att, priv)
	writeGovernanceAttestation(t, path, &att)
}

func writeGovernanceAttestation(t *testing.T, path string, att *governanceAttestation) {
	t.Helper()
	payload, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		t.Fatalf("marshal attestation: %v", err)
	}
	if err := os.WriteFile(path, payload, 0644); err != nil { //nolint:gosec // fixture path
		t.Fatalf("write attestation: %v", err)
	}
}

func signGovernanceAttestation(t *testing.T, att *governanceAttestation, priv ed25519.PrivateKey) {
	t.Helper()
	clone := *att
	clone.Signature = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		t.Fatalf("marshal attestation payload: %v", err)
	}
	sig := ed25519.Sign(priv, payload)
	att.Signature = base64.StdEncoding.EncodeToString(sig)
}
