package proxy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2EEchoThroughProxy is an end-to-end integration test that provisions a
// CA, starts the mTLS proxy server with "echo" allowed, builds the
// gt-proxy-client binary (symlinked as "echo"), and verifies that running the
// aliased echo command routes through the proxy and returns the expected output.
func TestE2EEchoThroughProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	// --- Provision CA ---
	caDir := t.TempDir()
	ca, err := GenerateCA(caDir)
	require.NoError(t, err)

	// --- Start proxy server with echo allowed ---
	srv, err := New(Config{
		ListenAddr:      "127.0.0.1:0",
		AllowedCommands: []string{"echo"},
		TownRoot:        t.TempDir(),
		Logger:          discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() { _ = srv.Start(ctx) }()

	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, addr, 5*time.Second)

	// --- Build gt-proxy-client and symlink as "echo" ---
	binDir := t.TempDir()
	clientBin := filepath.Join(binDir, "gt-proxy-client")

	buildCmd := exec.Command("go", "build", "-o", clientBin, "./cmd/gt-proxy-client")
	buildCmd.Dir = filepath.Join("..", "..") // repo root from internal/proxy/
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build gt-proxy-client failed:\n%s", buildOut)

	// Symlink client binary as "echo" so toolNameFromArg0 returns "echo".
	echoLink := filepath.Join(binDir, "echo")
	require.NoError(t, os.Symlink(clientBin, echoLink))

	// --- Issue client certificate ---
	certDir := t.TempDir()
	certPEM, keyPEM, err := ca.IssuePolecat("gt-gastown-e2etest", time.Hour)
	require.NoError(t, err)

	certFile := filepath.Join(certDir, "client.crt")
	keyFile := filepath.Join(certDir, "client.key")
	caFile := filepath.Join(certDir, "ca.crt")

	require.NoError(t, os.WriteFile(certFile, certPEM, 0644))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))
	require.NoError(t, os.WriteFile(caFile, ca.CertPEM, 0644))

	proxyEnv := []string{
		"GT_PROXY_URL=https://" + addr,
		"GT_PROXY_CERT=" + certFile,
		"GT_PROXY_KEY=" + keyFile,
		"GT_PROXY_CA=" + caFile,
	}

	// --- Run echo through the proxy ---

	t.Run("echo hello world", func(t *testing.T) {
		cmd := exec.Command(echoLink, "hello", "world")
		cmd.Env = proxyEnv
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "echo through proxy failed: %s", output)
		// Identity is passed via GT_PROXY_IDENTITY env var, not injected into argv.
		assert.Equal(t, "hello world\n", string(output))
	})

	t.Run("echo no args", func(t *testing.T) {
		cmd := exec.Command(echoLink)
		cmd.Env = proxyEnv
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "echo through proxy failed: %s", output)
		assert.Equal(t, "\n", string(output))
	})

	t.Run("echo multiple arguments preserves order", func(t *testing.T) {
		cmd := exec.Command(echoLink, "a", "b", "c", "d")
		cmd.Env = proxyEnv
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "echo through proxy failed: %s", output)
		assert.Equal(t, "a b c d\n", string(output))
	})

	t.Run("wrong CA cert is rejected at TLS handshake", func(t *testing.T) {
		// Issue a cert from a completely different CA.
		otherCA, err := GenerateCA(t.TempDir())
		require.NoError(t, err)
		wrongCertPEM, wrongKeyPEM, err := otherCA.IssuePolecat("gt-gastown-evil", time.Hour)
		require.NoError(t, err)

		wrongDir := t.TempDir()
		wrongCertFile := filepath.Join(wrongDir, "wrong.crt")
		wrongKeyFile := filepath.Join(wrongDir, "wrong.key")
		require.NoError(t, os.WriteFile(wrongCertFile, wrongCertPEM, 0644))
		require.NoError(t, os.WriteFile(wrongKeyFile, wrongKeyPEM, 0600))

		cmd := exec.Command(echoLink, "should", "fail")
		cmd.Env = []string{
			"GT_PROXY_URL=https://" + addr,
			"GT_PROXY_CERT=" + wrongCertFile,
			"GT_PROXY_KEY=" + wrongKeyFile,
			"GT_PROXY_CA=" + caFile, // still trust the real server cert
		}
		output, err := cmd.CombinedOutput()
		assert.Error(t, err, "wrong cert should cause TLS rejection")
		assert.Contains(t, string(output), "proxy request failed")
	})

	t.Run("missing proxy env falls back and fails without real binary", func(t *testing.T) {
		cmd := exec.Command(echoLink, "should", "fail")
		// No GT_PROXY_* vars → client falls back to execReal.
		cmd.Env = []string{"GT_REAL_BIN=/nonexistent/binary/xyzzy"}
		output, err := cmd.CombinedOutput()
		assert.Error(t, err, "should fail when falling back to nonexistent real binary")
		assert.Contains(t, string(output), "gt-proxy-client: exec")
	})
}
