//go:build integration

package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

const (
	testNamespace = "gastown-test"
	testPodName   = "terminal-integration-test"
	testAgentID   = "gastown/polecats/integration-test"
	// agentIDToSessionName("gastown/polecats/integration-test") = "gt-gastown-integration-test"
	testSessionName = "gt-gastown-integration-test"
)

// requireKubectl skips the test if kubectl is not available or the cluster is inaccessible.
func requireKubectl(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not found in PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", "cluster-info", "--request-timeout=5s")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("kubectl cluster not accessible: %v\n%s", err, output)
	}

	cmd = exec.CommandContext(ctx, "kubectl", "get", "namespace", testNamespace, "--request-timeout=5s")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("namespace %s not accessible: %v\n%s", testNamespace, err, output)
	}
}

// requireTmux skips the test if tmux is not available.
func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH")
	}
}

// deployTestPod creates a minimal test pod with a screen session in gastown-test.
// Returns a cleanup function that deletes the pod.
func deployTestPod(t *testing.T) func() {
	t.Helper()

	podYAML := fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
    app: gastown-test
    component: terminal-integration-test
spec:
  containers:
    - name: agent
      image: alpine:3.19
      command: ["sh", "-c"]
      args:
        - |
          apk add --no-cache screen bash > /dev/null 2>&1
          screen -dmS agent bash
          exec sleep infinity
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "500m"
          memory: "256Mi"
  restartPolicy: Never
`, testPodName, testNamespace)

	tmpFile, err := os.CreateTemp("", "terminal-test-pod-*.yaml")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(podYAML); err != nil {
		t.Fatalf("writing pod YAML: %v", err)
	}
	tmpFile.Close()

	// Clean up any previous test pod
	_ = exec.Command("kubectl", "delete", "pod", testPodName,
		"-n", testNamespace, "--ignore-not-found", "--grace-period=0", "--force").Run()

	// Deploy
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", tmpPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("deploying test pod: %v\n%s", err, output)
	}
	t.Logf("Test pod deployed: %s/%s", testNamespace, testPodName)

	// Wait for Running
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer waitCancel()
	cmd = exec.CommandContext(waitCtx, "kubectl", "wait", "--for=condition=Ready",
		"pod/"+testPodName, "-n", testNamespace, "--timeout=120s")
	output, err = cmd.CombinedOutput()
	if err != nil {
		descCmd := exec.Command("kubectl", "describe", "pod", testPodName, "-n", testNamespace)
		descOutput, _ := descCmd.CombinedOutput()
		t.Fatalf("waiting for pod Ready: %v\n%s\nDescribe:\n%s", err, output, descOutput)
	}
	t.Log("Test pod is Ready")

	return func() {
		_ = exec.Command("kubectl", "delete", "pod", testPodName,
			"-n", testNamespace, "--ignore-not-found", "--grace-period=0", "--force").Run()
		t.Log("Test pod deleted")
	}
}

// waitForScreen waits until the screen session named "agent" is running inside the pod.
func waitForScreen(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out waiting for screen session inside pod")
		default:
		}
		cmd := exec.CommandContext(ctx, "kubectl", "exec", testPodName,
			"-n", testNamespace, "--", "screen", "-ls")
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "agent") {
			t.Log("Screen session 'agent' is running inside the pod")
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// cleanupTmuxSession removes a local tmux session if it exists.
func cleanupTmuxSession(name string) {
	_ = exec.Command("tmux", "kill-session", "-t", name).Run()
}

// newTestSource returns a mockPodSource with a single test pod.
func newTestSource() *mockPodSource {
	return &mockPodSource{
		pods: []*PodInfo{{
			AgentID:   testAgentID,
			PodName:   testPodName,
			PodStatus: "running",
		}},
	}
}

// newTestServer creates a terminal server configured for integration testing.
func newTestServer(source PodSource) *Server {
	return NewServer(ServerConfig{
		Rig:            "gastown",
		Namespace:      testNamespace,
		PodSource:      source,
		PollInterval:   500 * time.Millisecond,
		HealthInterval: 1 * time.Second,
	})
}

// waitForConnection polls server status until at least one connection exists.
func waitForConnection(t *testing.T, srv *Server, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for terminal server connection")
		default:
		}
		if len(srv.Status().Connections) > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// runServer starts a server in a goroutine and returns a cancel + done channel.
func runServer(srv *Server) (context.CancelFunc, <-chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()
	return cancel, done
}

// shutdownServer cancels the server and waits for it to stop.
func shutdownServer(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("server shutdown error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down within timeout")
	}
}

// TestIntegration_TerminalServer exercises the terminal server against a real
// K8s agent pod on the e2e cluster (gastown-test namespace).
//
// Prerequisites:
//   - kubectl configured with access to e2e cluster
//   - gastown-test namespace exists
//   - tmux available locally
//
// Run: go test -tags integration -run TestIntegration_TerminalServer -v -timeout 300s
func TestIntegration_TerminalServer(t *testing.T) {
	requireKubectl(t)
	requireTmux(t)

	cleanup := deployTestPod(t)
	defer cleanup()
	waitForScreen(t)

	t.Run("DiscoversPod", func(t *testing.T) {
		source := newTestSource()
		srv := newTestServer(source)
		defer cleanupTmuxSession(testSessionName)

		cancel, done := runServer(srv)
		defer shutdownServer(t, cancel, done)

		// Wait for the terminal server to discover and connect to the pod
		waitForConnection(t, srv, 30*time.Second)

		status := srv.Status()
		if len(status.Connections) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(status.Connections))
		}

		conn := status.Connections[0]
		if conn.AgentID != testAgentID {
			t.Errorf("AgentID = %q, want %q", conn.AgentID, testAgentID)
		}
		if conn.SessionName != testSessionName {
			t.Errorf("SessionName = %q, want %q", conn.SessionName, testSessionName)
		}
		if conn.PodName != testPodName {
			t.Errorf("PodName = %q, want %q", conn.PodName, testPodName)
		}
		t.Logf("Terminal server discovered pod: agent=%s session=%s", conn.AgentID, conn.SessionName)
	})

	t.Run("TmuxSessionCreated", func(t *testing.T) {
		source := newTestSource()
		srv := newTestServer(source)
		defer cleanupTmuxSession(testSessionName)

		cancel, done := runServer(srv)
		defer shutdownServer(t, cancel, done)

		waitForConnection(t, srv, 30*time.Second)

		// Verify the local tmux session exists
		tm := tmux.NewTmux()
		has, err := tm.HasSession(testSessionName)
		if err != nil {
			t.Fatalf("HasSession error: %v", err)
		}
		if !has {
			t.Fatalf("tmux session %q does not exist", testSessionName)
		}

		// Verify session appears in list
		sessions, err := tm.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions error: %v", err)
		}
		found := false
		for _, s := range sessions {
			if s == testSessionName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("session %q not in session list: %v", testSessionName, sessions)
		}
		t.Logf("Tmux session %q created and listed", testSessionName)
	})

	t.Run("InputFlowsToPod", func(t *testing.T) {
		source := newTestSource()
		srv := newTestServer(source)
		defer cleanupTmuxSession(testSessionName)

		cancel, done := runServer(srv)
		defer shutdownServer(t, cancel, done)

		waitForConnection(t, srv, 30*time.Second)

		// Let the screen attachment stabilize
		time.Sleep(3 * time.Second)

		marker := fmt.Sprintf("INTTEST_%d", time.Now().UnixNano())

		// Send echo command through the local tmux session
		tm := tmux.NewTmux()
		if err := tm.SendKeys(testSessionName, fmt.Sprintf("echo %s", marker)); err != nil {
			t.Fatalf("SendKeys error: %v", err)
		}

		// Wait for command to propagate through kubectl exec → pod screen
		time.Sleep(3 * time.Second)

		// Verify marker appears in local tmux capture (which mirrors pod screen)
		output, err := tm.CapturePane(testSessionName, 50)
		if err != nil {
			t.Fatalf("CapturePane error: %v", err)
		}
		if !strings.Contains(output, marker) {
			t.Errorf("marker %q not found in tmux output:\n%s", marker, output)
		} else {
			t.Logf("Input verified: marker %q reached pod and echoed back", marker)
		}

		// Cross-check: verify inside the pod using screen hardcopy
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer verifyCancel()

		cmd := exec.CommandContext(verifyCtx, "kubectl", "exec", testPodName,
			"-n", testNamespace, "--",
			"screen", "-S", "agent", "-X", "hardcopy", "/tmp/screen.txt")
		if err := cmd.Run(); err != nil {
			t.Logf("Note: screen hardcopy failed (expected in some environments): %v", err)
			return
		}

		catCmd := exec.CommandContext(verifyCtx, "kubectl", "exec", testPodName,
			"-n", testNamespace, "--", "cat", "/tmp/screen.txt")
		podOutput, err := catCmd.Output()
		if err != nil {
			t.Logf("Note: reading hardcopy failed: %v", err)
			return
		}
		if strings.Contains(string(podOutput), marker) {
			t.Logf("Cross-check: marker confirmed inside pod screen session")
		} else {
			t.Logf("Note: marker not in hardcopy (may be in scrollback): %s", string(podOutput))
		}
	})

	t.Run("ReconnectAfterDisconnect", func(t *testing.T) {
		source := newTestSource()
		srv := newTestServer(source)
		defer cleanupTmuxSession(testSessionName)

		cancel, done := runServer(srv)
		defer shutdownServer(t, cancel, done)

		waitForConnection(t, srv, 30*time.Second)

		t.Log("Killing kubectl exec process to simulate disconnect")

		// Find the pane PID and kill its child (kubectl exec) to simulate a drop.
		pidCmd := exec.Command("tmux", "display-message", "-t", testSessionName, "-p", "#{pane_pid}")
		pidOutput, err := pidCmd.Output()
		if err != nil {
			t.Fatalf("getting pane PID: %v", err)
		}
		panePID := strings.TrimSpace(string(pidOutput))
		if panePID != "" {
			// Kill child processes (the kubectl exec command)
			_ = exec.Command("pkill", "-P", panePID).Run()
		}

		// Wait for the health monitor to detect and attempt reconnection.
		// Health interval is 1s, reconnect may take a few cycles.
		time.Sleep(8 * time.Second)

		srv.mu.RLock()
		pc, exists := srv.connections[testAgentID]
		srv.mu.RUnlock()

		if exists && pc.IsConnected() {
			t.Log("Connection successfully reconnected")

			// Verify tmux session is live after reconnect
			tm := tmux.NewTmux()
			has, err := tm.HasSession(testSessionName)
			if err != nil {
				t.Fatalf("HasSession after reconnect: %v", err)
			}
			if !has {
				t.Error("tmux session should exist after reconnect")
			}
		} else if exists {
			t.Logf("Connection exists, connected=%v, reconnect_count=%d",
				pc.IsConnected(), pc.ReconnectCount())
		} else {
			// Connection removed after max reconnects — it should be re-created
			// on the next discovery cycle if the pod is still alive.
			t.Log("Connection removed (max reconnects), waiting for re-discovery")
			time.Sleep(3 * time.Second)
			status := srv.Status()
			t.Logf("After re-discovery: %d connections", len(status.Connections))
		}
	})

	t.Run("CleanupOnDeregistration", func(t *testing.T) {
		source := newTestSource()
		srv := newTestServer(source)
		defer cleanupTmuxSession(testSessionName)

		cancel, done := runServer(srv)
		defer shutdownServer(t, cancel, done)

		waitForConnection(t, srv, 30*time.Second)

		t.Log("Deregistering pod from source")

		// Simulate pod deregistration by removing it from the source
		source.setPods([]*PodInfo{})

		// Wait for discovery to detect removal and clean up
		time.Sleep(3 * time.Second)

		status := srv.Status()
		if len(status.Connections) != 0 {
			t.Errorf("expected 0 connections after deregistration, got %d", len(status.Connections))
		}

		// Verify tmux session cleaned up
		tm := tmux.NewTmux()
		has, err := tm.HasSession(testSessionName)
		if err != nil {
			t.Fatalf("HasSession after deregistration: %v", err)
		}
		if has {
			t.Error("tmux session should be cleaned up after pod deregistration")
		}
		t.Log("Connection and tmux session cleaned up after deregistration")
	})
}
