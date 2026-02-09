package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/steveyegge/gastown/controller/internal/beadswatcher"
	"github.com/steveyegge/gastown/controller/internal/config"
	"github.com/steveyegge/gastown/controller/internal/podmanager"
)

func TestBuildAgentPodSpec_CoopSidecarFromConfig(t *testing.T) {
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}

	spec := buildAgentPodSpec(cfg, event)

	if spec.CoopSidecar == nil {
		t.Fatal("expected CoopSidecar to be set when CoopImage is configured")
	}
	if spec.CoopSidecar.Image != "ghcr.io/groblegark/coop:latest" {
		t.Errorf("CoopSidecar.Image = %q, want %q", spec.CoopSidecar.Image, "ghcr.io/groblegark/coop:latest")
	}
}

func TestBuildAgentPodSpec_NoCoopWithoutImage(t *testing.T) {
	cfg := &config.Config{
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}

	spec := buildAgentPodSpec(cfg, event)

	if spec.CoopSidecar != nil {
		t.Fatal("expected CoopSidecar to be nil when CoopImage is empty")
	}
}

func TestBuildAgentPodSpec_CoopNatsFromMetadata(t *testing.T) {
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "crew",
		AgentName: "k8s",
		BeadID:    "gt-test-456",
		Metadata: map[string]string{
			"image":             "agent:latest",
			"nats_url":          "nats://bd-daemon:4222",
			"nats_token_secret": "gastown-nats-token",
			"coop_auth_secret":  "gastown-coop-auth",
		},
	}

	spec := buildAgentPodSpec(cfg, event)

	if spec.CoopSidecar == nil {
		t.Fatal("expected CoopSidecar to be set")
	}
	if spec.CoopSidecar.NatsURL != "nats://bd-daemon:4222" {
		t.Errorf("NatsURL = %q, want %q", spec.CoopSidecar.NatsURL, "nats://bd-daemon:4222")
	}
	if spec.CoopSidecar.NatsTokenSecret != "gastown-nats-token" {
		t.Errorf("NatsTokenSecret = %q, want %q", spec.CoopSidecar.NatsTokenSecret, "gastown-nats-token")
	}
	if spec.CoopSidecar.AuthTokenSecret != "gastown-coop-auth" {
		t.Errorf("AuthTokenSecret = %q, want %q", spec.CoopSidecar.AuthTokenSecret, "gastown-coop-auth")
	}
}

func TestBuildAgentPodSpec_CoopMetadataAvailableForReporting(t *testing.T) {
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}

	spec := buildAgentPodSpec(cfg, event)

	// Verify the spec has what handleEvent needs for backend metadata reporting.
	if spec.CoopSidecar == nil {
		t.Fatal("CoopSidecar must be set for metadata reporting")
	}
	expectedPodName := "gt-gastown-polecat-furiosa"
	if spec.PodName() != expectedPodName {
		t.Errorf("PodName() = %q, want %q", spec.PodName(), expectedPodName)
	}
	if spec.Namespace != "gastown" {
		t.Errorf("Namespace = %q, want %q", spec.Namespace, "gastown")
	}
}

func TestBuildAgentPodSpec_BasicFields(t *testing.T) {
	cfg := &config.Config{
		Namespace:    "gastown",
		DaemonHost:   "bd-daemon",
		DaemonPort:   9876,
		DefaultImage: "default:latest",
	}
	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "rictus",
		BeadID:    "gt-test-789",
		Metadata:  map[string]string{"image": "agent:v1"},
	}

	spec := buildAgentPodSpec(cfg, event)

	if spec.Rig != "gastown" {
		t.Errorf("Rig = %q, want %q", spec.Rig, "gastown")
	}
	if spec.Role != "polecat" {
		t.Errorf("Role = %q, want %q", spec.Role, "polecat")
	}
	if spec.AgentName != "rictus" {
		t.Errorf("AgentName = %q, want %q", spec.AgentName, "rictus")
	}
	if spec.Namespace != "gastown" {
		t.Errorf("Namespace = %q, want %q", spec.Namespace, "gastown")
	}
	if spec.PodName() != "gt-gastown-polecat-rictus" {
		t.Errorf("PodName() = %q, want %q", spec.PodName(), "gt-gastown-polecat-rictus")
	}
}

func TestHandleEvent_SpawnWithCoopReportsBackendMetadata(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := slog.Default()
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	pods := podmanager.New(client, logger)
	reporter := newRecordingReporter(client, cfg.Namespace, logger)
	ctx := context.Background()

	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}

	if err := handleEvent(ctx, logger, cfg, event, pods, reporter); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	// Verify pod was created.
	_, err := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-furiosa", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("pod not created: %v", err)
	}

	// Verify backend metadata was reported.
	meta := reporter.BackendMeta()
	if len(meta) != 1 {
		t.Fatalf("expected 1 backend metadata report, got %d", len(meta))
	}
	if meta[0].Meta.Backend != "coop" {
		t.Errorf("Backend = %q, want %q", meta[0].Meta.Backend, "coop")
	}
	if meta[0].Meta.PodName != "gt-gastown-polecat-furiosa" {
		t.Errorf("PodName = %q, want %q", meta[0].Meta.PodName, "gt-gastown-polecat-furiosa")
	}
	expectedURL := "http://gt-gastown-polecat-furiosa.gastown.svc.cluster.local:8080"
	if meta[0].Meta.CoopURL != expectedURL {
		t.Errorf("CoopURL = %q, want %q", meta[0].Meta.CoopURL, expectedURL)
	}
	if meta[0].AgentName != "gt-gastown-polecat-furiosa" {
		t.Errorf("AgentName = %q, want %q", meta[0].AgentName, "gt-gastown-polecat-furiosa")
	}
}

func TestHandleEvent_SpawnWithoutCoopSkipsBackendMetadata(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := slog.Default()
	cfg := testConfig() // No CoopImage
	pods := podmanager.New(client, logger)
	reporter := newRecordingReporter(client, cfg.Namespace, logger)
	ctx := context.Background()

	event := spawnEvent("gastown", "polecat", "furiosa")
	if err := handleEvent(ctx, logger, cfg, event, pods, reporter); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	meta := reporter.BackendMeta()
	if len(meta) != 0 {
		t.Errorf("expected no backend metadata reports without CoopImage, got %d", len(meta))
	}
}

func TestHandleEvent_DoneClearsBackendMetadata(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := slog.Default()
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	pods := podmanager.New(client, logger)
	reporter := newRecordingReporter(client, cfg.Namespace, logger)
	ctx := context.Background()

	// Spawn first.
	spawnEvt := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}
	if err := handleEvent(ctx, logger, cfg, spawnEvt, pods, reporter); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// Done should clear metadata.
	doneEvt := doneEvent("gastown", "polecat", "furiosa")
	if err := handleEvent(ctx, logger, cfg, doneEvt, pods, reporter); err != nil {
		t.Fatalf("done: %v", err)
	}

	// Should have 2 backend metadata reports: one with coop, one clearing.
	meta := reporter.BackendMeta()
	if len(meta) != 2 {
		t.Fatalf("expected 2 backend metadata reports, got %d", len(meta))
	}
	// First should be coop setup.
	if meta[0].Meta.Backend != "coop" {
		t.Errorf("first report Backend = %q, want %q", meta[0].Meta.Backend, "coop")
	}
	// Second should be clear (empty backend).
	if meta[1].Meta.Backend != "" {
		t.Errorf("second report Backend = %q, want empty (clear)", meta[1].Meta.Backend)
	}
}

func TestHandleEvent_CoopURLIncludesCustomPort(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := slog.Default()
	cfg := &config.Config{
		CoopImage:  "ghcr.io/groblegark/coop:latest",
		Namespace:  "gastown",
		DaemonHost: "localhost",
		DaemonPort: 9876,
	}
	pods := podmanager.New(client, logger)
	reporter := newRecordingReporter(client, cfg.Namespace, logger)
	ctx := context.Background()

	event := beadswatcher.Event{
		Type:      beadswatcher.AgentSpawn,
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		BeadID:    "gt-test-123",
		Metadata:  map[string]string{"image": "agent:latest"},
	}

	if err := handleEvent(ctx, logger, cfg, event, pods, reporter); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	meta := reporter.BackendMeta()
	if len(meta) < 1 {
		t.Fatal("no backend metadata reported")
	}
	// Default port should be 8080.
	if !strings.Contains(meta[0].Meta.CoopURL, ":8080") {
		t.Errorf("CoopURL = %q, should contain default port :8080", meta[0].Meta.CoopURL)
	}
}
