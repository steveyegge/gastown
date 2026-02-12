package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/steveyegge/gastown/controller/internal/config"
	"github.com/steveyegge/gastown/controller/internal/daemonclient"
	"github.com/steveyegge/gastown/controller/internal/podmanager"
)

// ---------------------------------------------------------------------------
// Mock daemon client
// ---------------------------------------------------------------------------

type mockBeadLister struct {
	beads []daemonclient.AgentBead
	err   error
}

func (m *mockBeadLister) ListAgentBeads(ctx context.Context) ([]daemonclient.AgentBead, error) {
	return m.beads, m.err
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const testNamespace = "gastown"

func testCfg() *config.Config {
	return &config.Config{
		Namespace:    testNamespace,
		DaemonHost:   "localhost",
		DaemonPort:   9876,
		DefaultImage: "gastown-agent:test",
	}
}

// testSpecBuilder returns a minimal AgentPodSpec. The reconciler delegates
// pod construction to this function; we only need enough for the fake client.
func testSpecBuilder(cfg *config.Config, rig, role, agentName string, _ map[string]string) podmanager.AgentPodSpec {
	return podmanager.AgentPodSpec{
		Rig:       rig,
		Role:      role,
		AgentName: agentName,
		Image:     cfg.DefaultImage,
		Namespace: cfg.Namespace,
	}
}

// createFakePod inserts a pre-existing pod into the fake K8s client with the
// given name, namespace, phase, and standard gastown labels parsed from the name.
func createFakePod(t *testing.T, client kubernetes.Interface, name, namespace, phase string) {
	t.Helper()
	rig, role, agent := parsePodName(t, name)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				podmanager.LabelApp:   podmanager.LabelAppValue,
				podmanager.LabelRig:   rig,
				podmanager.LabelRole:  role,
				podmanager.LabelAgent: agent,
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPhase(phase),
		},
	}
	_, err := client.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating fake pod %s: %v", name, err)
	}
}

// parsePodName extracts rig, role, agent from a pod name like "gt-town-mayor-hq".
func parsePodName(t *testing.T, name string) (rig, role, agent string) {
	t.Helper()
	// Pod name format: gt-{rig}-{role}-{agent}
	// Simple split — works for single-segment names in tests.
	var parts []string
	current := ""
	prefix := "gt-"
	if len(name) <= len(prefix) {
		t.Fatalf("invalid pod name %q: too short", name)
	}
	rest := name[len(prefix):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '-' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(rest[i])
		}
	}
	parts = append(parts, current)

	if len(parts) < 3 {
		t.Fatalf("invalid pod name %q: expected at least 3 parts after gt-, got %v", name, parts)
	}
	return parts[0], parts[1], joinParts(parts[2:])
}

func joinParts(parts []string) string {
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "-" + parts[i]
	}
	return result
}

// listPodNames returns sorted names of all pods in the namespace.
func listPodNames(t *testing.T, client kubernetes.Interface, namespace string) []string {
	t.Helper()
	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("listing pods: %v", err)
	}
	names := make([]string, 0, len(pods.Items))
	for _, p := range pods.Items {
		names = append(names, p.Name)
	}
	return names
}

// bead creates a test AgentBead with the given identity.
func bead(rig, role, agent string) daemonclient.AgentBead {
	return daemonclient.AgentBead{
		ID:        fmt.Sprintf("%s-%s", rig, agent),
		Rig:       rig,
		Role:      role,
		AgentName: agent,
	}
}

// newReconciler creates a Reconciler backed by a fake K8s client and mock daemon.
func newReconciler(client kubernetes.Interface, beads []daemonclient.AgentBead, beadErr error) *Reconciler {
	logger := slog.Default()
	cfg := testCfg()
	lister := &mockBeadLister{beads: beads, err: beadErr}
	pods := podmanager.New(client, logger)
	return New(lister, pods, cfg, logger, testSpecBuilder)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReconcile_CreatesMissingPod(t *testing.T) {
	// One bead (hq-mayor), no pods -> pod gt-town-mayor-hq created.
	client := fake.NewSimpleClientset()
	r := newReconciler(client, []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 {
		t.Fatalf("expected 1 pod, got %d: %v", len(names), names)
	}
	if names[0] != "gt-town-mayor-hq" {
		t.Errorf("pod name = %q, want %q", names[0], "gt-town-mayor-hq")
	}
}

func TestReconcile_DeletesOrphanPod(t *testing.T) {
	// No beads, one pod gt-town-mayor-hq -> pod deleted.
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Running")

	r := newReconciler(client, nil, nil)
	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 0 {
		t.Errorf("expected 0 pods after deleting orphan, got %d: %v", len(names), names)
	}
}

func TestReconcile_NoOpWhenMatching(t *testing.T) {
	// One bead + matching Running pod -> no creates/deletes.
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Running")

	r := newReconciler(client, []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 {
		t.Fatalf("expected 1 pod (unchanged), got %d: %v", len(names), names)
	}
	if names[0] != "gt-town-mayor-hq" {
		t.Errorf("pod name = %q, want %q", names[0], "gt-town-mayor-hq")
	}
}

func TestReconcile_DeletesAndRecreatesFailedPod(t *testing.T) {
	// Bead exists, pod in Failed phase -> pod deleted and recreated.
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Failed")

	r := newReconciler(client, []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 {
		t.Fatalf("expected 1 pod (recreated), got %d: %v", len(names), names)
	}
	if names[0] != "gt-town-mayor-hq" {
		t.Errorf("pod name = %q, want %q", names[0], "gt-town-mayor-hq")
	}

	// Verify it's a new pod (the failed one was deleted and a new one created).
	// The fake client doesn't track phases from create, so just verify it exists.
	pod, err := client.CoreV1().Pods(testNamespace).Get(ctx, "gt-town-mayor-hq", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected pod to exist after recreation: %v", err)
	}
	// Newly created pod won't have the Failed phase (fake client creates with empty phase).
	if pod.Status.Phase == corev1.PodFailed {
		t.Error("recreated pod should not be in Failed phase")
	}
}

func TestReconcile_SkipsPendingPod(t *testing.T) {
	// Bead exists, pod in Pending phase -> no action (still starting).
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-gastown-crew-k8s", testNamespace, "Pending")

	r := newReconciler(client, []daemonclient.AgentBead{
		bead("gastown", "crew", "k8s"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 {
		t.Fatalf("expected 1 pod (unchanged), got %d: %v", len(names), names)
	}

	// Verify phase is still Pending (not recreated).
	pod, err := client.CoreV1().Pods(testNamespace).Get(ctx, "gt-gastown-crew-k8s", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if pod.Status.Phase != corev1.PodPending {
		t.Errorf("pod phase = %q, want Pending (should be untouched)", pod.Status.Phase)
	}
}

func TestReconcile_EmptyDesiredDeletesAll(t *testing.T) {
	// No beads, multiple pods -> all pods deleted.
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Running")
	createFakePod(t, client, "gt-gastown-crew-k8s", testNamespace, "Running")
	createFakePod(t, client, "gt-gastown-polecat-furiosa", testNamespace, "Running")

	r := newReconciler(client, nil, nil)
	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 0 {
		t.Errorf("expected 0 pods, got %d: %v", len(names), names)
	}
}

func TestReconcile_EmptyActualCreatesAll(t *testing.T) {
	// Multiple beads, no pods -> all pods created.
	client := fake.NewSimpleClientset()
	r := newReconciler(client, []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
		bead("gastown", "crew", "k8s"),
		bead("gastown", "polecat", "furiosa"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 3 {
		t.Fatalf("expected 3 pods, got %d: %v", len(names), names)
	}

	expected := map[string]bool{
		"gt-town-mayor-hq":             true,
		"gt-gastown-crew-k8s":          true,
		"gt-gastown-polecat-furiosa":   true,
	}
	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected pod %q", n)
		}
		delete(expected, n)
	}
	for n := range expected {
		t.Errorf("missing expected pod %q", n)
	}
}

func TestReconcile_DaemonError_FailSafe(t *testing.T) {
	// Daemon returns error -> no pods deleted (fail-safe), error returned.
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Running")
	createFakePod(t, client, "gt-gastown-crew-k8s", testNamespace, "Running")

	r := newReconciler(client, nil, fmt.Errorf("connection refused"))
	ctx := context.Background()
	err := r.Reconcile(ctx)
	if err == nil {
		t.Fatal("expected error from Reconcile when daemon fails")
	}

	// Pods must still exist (fail-safe: don't delete on daemon error).
	names := listPodNames(t, client, testNamespace)
	if len(names) != 2 {
		t.Errorf("expected 2 pods preserved (fail-safe), got %d: %v", len(names), names)
	}
}

func TestReconcile_MultipleBeadsAndPods(t *testing.T) {
	// 3 beads, 3 pods:
	// - gt-town-mayor-hq: bead exists, pod Running -> no-op
	// - gt-gastown-crew-k8s: no bead -> delete orphan
	// - gt-gastown-polecat-furiosa: bead exists, no pod -> create
	client := fake.NewSimpleClientset()
	createFakePod(t, client, "gt-town-mayor-hq", testNamespace, "Running")
	createFakePod(t, client, "gt-gastown-crew-k8s", testNamespace, "Running")

	r := newReconciler(client, []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
		bead("gastown", "polecat", "furiosa"),
	}, nil)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 2 {
		t.Fatalf("expected 2 pods, got %d: %v", len(names), names)
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	// Should still exist (matching bead).
	if !nameSet["gt-town-mayor-hq"] {
		t.Error("gt-town-mayor-hq should still exist (matching bead)")
	}
	// Should have been deleted (orphan).
	if nameSet["gt-gastown-crew-k8s"] {
		t.Error("gt-gastown-crew-k8s should have been deleted (orphan)")
	}
	// Should have been created (missing pod).
	if !nameSet["gt-gastown-polecat-furiosa"] {
		t.Error("gt-gastown-polecat-furiosa should have been created (missing)")
	}
}

func TestReconcile_IgnoresPodsWithoutAgentLabel(t *testing.T) {
	// A pod with the gastown app label but no gastown.io/agent label should
	// NOT be treated as an agent pod (e.g., the controller itself).
	client := fake.NewSimpleClientset()

	// Create a pod that looks like the controller — has app label but no agent label.
	controllerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gastown-uat-agent-controller-abc123",
			Namespace: testNamespace,
			Labels: map[string]string{
				podmanager.LabelApp: podmanager.LabelAppValue,
				// No LabelAgent, LabelRig, or LabelRole
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	_, err := client.CoreV1().Pods(testNamespace).Create(context.Background(), controllerPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// No desired beads -> reconciler should NOT delete the controller pod.
	r := newReconciler(client, nil, nil)
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 || names[0] != "gastown-uat-agent-controller-abc123" {
		t.Errorf("controller pod should be preserved, got pods: %v", names)
	}
}

// --- sidecarChanged tests ---

func TestSidecarChanged_BothNil(t *testing.T) {
	pod := &corev1.Pod{}
	if sidecarChanged(nil, pod) {
		t.Error("sidecarChanged(nil, no init containers) should be false")
	}
}

func TestSidecarChanged_DesiredNil(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: podmanager.ToolchainContainerName, Image: "toolchain:v1"},
			},
		},
	}
	if !sidecarChanged(nil, pod) {
		t.Error("sidecarChanged(nil, existing sidecar) should be true (removal)")
	}
}

func TestSidecarChanged_CurrentNil(t *testing.T) {
	pod := &corev1.Pod{}
	desired := &podmanager.ToolchainSidecarSpec{Image: "toolchain:v1"}
	if !sidecarChanged(desired, pod) {
		t.Error("sidecarChanged(desired, no sidecar) should be true (addition)")
	}
}

func TestSidecarChanged_SameImage(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: podmanager.ToolchainContainerName, Image: "toolchain:v1"},
			},
		},
	}
	desired := &podmanager.ToolchainSidecarSpec{Image: "toolchain:v1"}
	if sidecarChanged(desired, pod) {
		t.Error("sidecarChanged with same image should be false")
	}
}

func TestSidecarChanged_DifferentImage(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: podmanager.ToolchainContainerName, Image: "toolchain:v1"},
			},
		},
	}
	desired := &podmanager.ToolchainSidecarSpec{Image: "toolchain:v2"}
	if !sidecarChanged(desired, pod) {
		t.Error("sidecarChanged with different image should be true")
	}
}

func TestReconcile_SidecarChangeTriggersPodRecreation(t *testing.T) {
	client := fake.NewSimpleClientset()

	// Create a pod with a toolchain init container (v1).
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gt-town-crew-worker",
			Namespace: testNamespace,
			Labels: map[string]string{
				podmanager.LabelApp:   podmanager.LabelAppValue,
				podmanager.LabelRig:   "town",
				podmanager.LabelRole:  "crew",
				podmanager.LabelAgent: "worker",
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: podmanager.ToolchainContainerName, Image: "toolchain:v1"},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	_, err := client.CoreV1().Pods(testNamespace).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Spec builder returns a spec with toolchain sidecar v2 (changed).
	specBuilder := func(cfg *config.Config, rig, role, agentName string, _ map[string]string) podmanager.AgentPodSpec {
		return podmanager.AgentPodSpec{
			Rig:       rig,
			Role:      role,
			AgentName: agentName,
			Image:     cfg.DefaultImage,
			Namespace: cfg.Namespace,
			ToolchainSidecar: &podmanager.ToolchainSidecarSpec{
				Image: "toolchain:v2",
			},
		}
	}

	cfg := testCfg()
	lister := &mockBeadLister{beads: []daemonclient.AgentBead{
		bead("town", "crew", "worker"),
	}}
	pods := podmanager.New(client, slog.Default())
	r := New(lister, pods, cfg, slog.Default(), specBuilder)

	ctx := context.Background()
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// The pod should have been deleted and recreated.
	names := listPodNames(t, client, testNamespace)
	if len(names) != 1 {
		t.Fatalf("expected 1 pod, got %d: %v", len(names), names)
	}
	if names[0] != "gt-town-crew-worker" {
		t.Errorf("pod name = %q, want %q", names[0], "gt-town-crew-worker")
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	// Call Reconcile twice with same state -> second call produces no extra operations.
	client := fake.NewSimpleClientset()
	beads := []daemonclient.AgentBead{
		bead("town", "mayor", "hq"),
		bead("gastown", "crew", "k8s"),
	}

	r := newReconciler(client, beads, nil)
	ctx := context.Background()

	// First reconciliation: creates both pods.
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	firstNames := listPodNames(t, client, testNamespace)
	if len(firstNames) != 2 {
		t.Fatalf("after first reconcile: expected 2 pods, got %d", len(firstNames))
	}

	// Pods created by the fake client have empty phase. The reconciler should
	// treat empty phase same as Pending (not Failed), so no recreation.
	// Set phases to Running to be explicit.
	for _, name := range firstNames {
		pod, _ := client.CoreV1().Pods(testNamespace).Get(ctx, name, metav1.GetOptions{})
		pod.Status.Phase = corev1.PodRunning
		client.CoreV1().Pods(testNamespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	}

	// Second reconciliation: should be a no-op.
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	secondNames := listPodNames(t, client, testNamespace)
	if len(secondNames) != 2 {
		t.Fatalf("after second reconcile: expected 2 pods, got %d: %v", len(secondNames), secondNames)
	}

	// Verify exact same pods exist.
	nameSet := make(map[string]bool)
	for _, n := range secondNames {
		nameSet[n] = true
	}
	if !nameSet["gt-town-mayor-hq"] {
		t.Error("gt-town-mayor-hq should still exist")
	}
	if !nameSet["gt-gastown-crew-k8s"] {
		t.Error("gt-gastown-crew-k8s should still exist")
	}
}
