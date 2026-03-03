package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pktLine encodes s as a single git pkt-line (4-hex-byte length prefix).
// The length field includes the 4 bytes of the length prefix itself.
func pktLine(s string) string {
	return fmt.Sprintf("%04x%s", len(s)+4, s)
}

// receivePackBody builds a minimal git-receive-pack pkt-line stream for the given refs.
// Each entry is formatted as "<zero-sha> <ones-sha> <refname>\n".
func receivePackBody(refs ...string) []byte {
	zeroSHA := strings.Repeat("0", 40)
	newSHA := strings.Repeat("a", 40)
	var buf strings.Builder
	for _, ref := range refs {
		line := zeroSHA + " " + newSHA + " " + ref + "\n"
		buf.WriteString(pktLine(line))
	}
	buf.WriteString("0000") // flush packet
	return []byte(buf.String())
}

// errReadCloser is an io.ReadCloser that always returns an error on Read.
type errReadCloser struct{ err error }

func (e errReadCloser) Read(_ []byte) (int, error) { return 0, e.err }
func (e errReadCloser) Close() error               { return nil }

// ---- validateReceivePackRefs ----

func TestValidateReceivePackRefs(t *testing.T) {
	const polecat = "furiosa"

	t.Run("empty body returns nil", func(t *testing.T) {
		assert.NoError(t, validateReceivePackRefs(nil, polecat))
		assert.NoError(t, validateReceivePackRefs([]byte{}, polecat))
	})

	t.Run("flush-only body returns nil", func(t *testing.T) {
		assert.NoError(t, validateReceivePackRefs([]byte("0000"), polecat))
	})

	t.Run("single valid ref returns nil", func(t *testing.T) {
		body := receivePackBody("refs/heads/polecat/furiosa-abc123")
		assert.NoError(t, validateReceivePackRefs(body, polecat))
	})

	t.Run("multiple valid refs return nil", func(t *testing.T) {
		body := receivePackBody(
			"refs/heads/polecat/furiosa-abc123",
			"refs/heads/polecat/furiosa-def456",
		)
		assert.NoError(t, validateReceivePackRefs(body, polecat))
	})

	t.Run("ref to main is denied", func(t *testing.T) {
		body := receivePackBody("refs/heads/main")
		err := validateReceivePackRefs(body, polecat)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "main")
	})

	t.Run("exact polecat ref without dash suffix is denied", func(t *testing.T) {
		// "refs/heads/polecat/furiosa" has no dash suffix → denied.
		body := receivePackBody("refs/heads/polecat/furiosa")
		err := validateReceivePackRefs(body, polecat)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refs/heads/polecat/furiosa")
	})

	t.Run("wrong polecat name is denied", func(t *testing.T) {
		body := receivePackBody("refs/heads/polecat/otherpolecat-abc")
		err := validateReceivePackRefs(body, polecat)
		require.Error(t, err)
	})

	t.Run("mixed valid and invalid returns error on first invalid", func(t *testing.T) {
		body := receivePackBody(
			"refs/heads/polecat/furiosa-ok",
			"refs/heads/main",
		)
		err := validateReceivePackRefs(body, polecat)
		assert.Error(t, err)
	})

	t.Run("malformed pkt-line length stops parsing without panic", func(t *testing.T) {
		// Only 2 bytes where 4 are needed for the length field.
		body := []byte("00")
		var err error
		assert.NotPanics(t, func() {
			err = validateReceivePackRefs(body, polecat)
		})
		require.Error(t, err, "truncated length field must be rejected (fail-closed)")
	})

	t.Run("truncated pkt-line body stops parsing without panic", func(t *testing.T) {
		// Length says 16 bytes but only "hello" (5 bytes payload) is present.
		body := []byte("0010hello")
		var err error
		assert.NotPanics(t, func() {
			err = validateReceivePackRefs(body, polecat)
		})
		require.Error(t, err, "truncated pkt-line body must be rejected (fail-closed)")
	})

	t.Run("pkt-line with NUL-separated capabilities parses ref correctly", func(t *testing.T) {
		zeroSHA := strings.Repeat("0", 40)
		newSHA := strings.Repeat("a", 40)
		// ref line with NUL-separated capability string
		line := zeroSHA + " " + newSHA + " refs/heads/polecat/furiosa-abc\x00side-band-64k\n"
		body := []byte(pktLine(line) + "0000")
		assert.NoError(t, validateReceivePackRefs(body, polecat))
	})

	t.Run("line with fewer than 3 fields is skipped without error", func(t *testing.T) {
		line := "onlyone\n"
		body := []byte(pktLine(line) + "0000")
		assert.NoError(t, validateReceivePackRefs(body, polecat))
	})

	t.Run("pktLen==4 empty payload does not spin", func(t *testing.T) {
		// "0004" means a packet with only the length field (no payload).
		body := []byte("0004" + "0000")
		assert.NotPanics(t, func() {
			_ = validateReceivePackRefs(body, polecat)
		})
	})

	t.Run("binary pack data after flush packet is ignored", func(t *testing.T) {
		// "0000" flush followed by raw binary pack data. Must not panic or error.
		binaryJunk := []byte("0000\x00\x00\x00\x02\xff\xfe\xfd\xfc PACK binary garbage")
		assert.NotPanics(t, func() {
			err := validateReceivePackRefs(binaryJunk, polecat)
			assert.NoError(t, err)
		})
	})
}

// ---- authorizeReceivePack ----

func TestAuthorizeReceivePack(t *testing.T) {
	srv, err := New(Config{TownRoot: t.TempDir()}, nil)
	require.NoError(t, err)

	t.Run("CN with no gt- prefix returns 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack",
			bytes.NewReader(receivePackBody()))
		ok, _ := srv.authorizeReceivePack(rec, req, "notgt-rig-name")
		assert.False(t, ok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("CN = 'gt-' with no rig or name returns 403", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack",
			bytes.NewReader(receivePackBody()))
		ok, _ := srv.authorizeReceivePack(rec, req, "gt-")
		assert.False(t, ok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("CN with empty rig segment returns 403", func(t *testing.T) {
		// gt--furiosa has an empty rig; polecatName now returns "" for this case.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack",
			bytes.NewReader(receivePackBody("refs/heads/polecat/furiosa-abc")))
		ok, _ := srv.authorizeReceivePack(rec, req, "gt--furiosa")
		assert.False(t, ok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("valid CN and valid refs — body is rewound correctly", func(t *testing.T) {
		cn := "gt-gastown-furiosa"
		body := receivePackBody("refs/heads/polecat/furiosa-abc123")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack",
			bytes.NewReader(body))
		ok, refs := srv.authorizeReceivePack(rec, req, cn)

		require.True(t, ok)
		assert.Equal(t, []string{"refs/heads/polecat/furiosa-abc123"}, refs)
		// Verify body was rewound so git can re-read it.
		rewound, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Equal(t, body, rewound, "body should be rewound to its original content")
	})

	t.Run("valid CN with invalid refs returns 403 and includes refs", func(t *testing.T) {
		cn := "gt-gastown-furiosa"
		body := receivePackBody("refs/heads/main")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack",
			bytes.NewReader(body))
		ok, refs := srv.authorizeReceivePack(rec, req, cn)

		assert.False(t, ok)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		// Refs are still returned even on denial (for audit logging).
		assert.Equal(t, []string{"refs/heads/main"}, refs)
	})

	t.Run("body read error returns 400", func(t *testing.T) {
		cn := "gt-gastown-furiosa"
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack", nil)
		req.Body = errReadCloser{err: fmt.Errorf("simulated read error")}

		ok, refs := srv.authorizeReceivePack(rec, req, cn)
		assert.False(t, ok)
		assert.Nil(t, refs)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("oversized body returns 400", func(t *testing.T) {
		cn := "gt-gastown-furiosa"
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/git/rig/git-receive-pack", nil)
		req.Body = errReadCloser{err: &http.MaxBytesError{Limit: 32 << 20}}

		ok, _ := srv.authorizeReceivePack(rec, req, cn)
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ---- handleGit routing ----

func newGitServer(t *testing.T) (*Server, string) {
	t.Helper()
	townRoot := t.TempDir()
	srv, err := New(Config{TownRoot: townRoot}, nil)
	require.NoError(t, err)
	// Pre-create the "testrip" repo directory so routing tests that reach
	// handleInfoRefs/handlePack pass the repo existence pre-flight.
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "testrip", ".repo.git"), 0700))
	return srv, townRoot
}

func TestHandleGitRouting(t *testing.T) {
	t.Run("no rig segment returns 400", func(t *testing.T) {
		srv, _ := newGitServer(t)
		// /v1/git/ with nothing after → path = "" → only one part
		req := httptest.NewRequest("GET", "/v1/git/", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("known rig with empty operation returns 404", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git/testrip/", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("unknown operation returns 404", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git/testrip/bogus", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("info/refs POST returns 405", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("POST", "/v1/git/testrip/info/refs?service=git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("info/refs with unsupported service returns 400", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git/testrip/info/refs?service=git-archive", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("git-receive-pack POST with no CN returns 403", func(t *testing.T) {
		srv, _ := newGitServer(t)
		body := receivePackBody("refs/heads/polecat/furiosa-abc")
		req := httptest.NewRequest("POST", "/v1/git/testrip/git-receive-pack",
			bytes.NewReader(body))
		// r.TLS is nil → clientCN returns "" → authorizeReceivePack returns 403
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("git-receive-pack GET returns 405", func(t *testing.T) {
		srv, _ := newGitServer(t)
		// handlePack checks method before auth, so a GET always returns 405.
		req := httptest.NewRequest("GET", "/v1/git/testrip/git-receive-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("git-upload-pack GET returns 405", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git/testrip/git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("double-slash produces empty rig and returns 400", func(t *testing.T) {
		// /v1/git//info/refs — rig segment is the empty string between the two slashes.
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git//info/refs?service=git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("path traversal in rig name returns 400", func(t *testing.T) {
		srv, _ := newGitServer(t)
		// URL-encoded ".." traversal attempt.
		req := httptest.NewRequest("GET", "/v1/git/..%2Fetc%2Fpasswd/info/refs?service=git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rig with invalid characters returns 400", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET", "/v1/git/rig@bad!/info/refs?service=git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestClientCN exercises all branches of the clientCN helper.
func TestClientCN(t *testing.T) {
	t.Run("nil TLS returns empty string", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		assert.Equal(t, "", clientCN(req))
	})

	t.Run("empty PeerCertificates returns empty string", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		req.TLS = &tls.ConnectionState{}
		assert.Equal(t, "", clientCN(req))
	})

	t.Run("cert with CN returns the CN", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Subject: pkix.Name{CommonName: "gt-gastown-furiosa"}},
			},
		}
		assert.Equal(t, "gt-gastown-furiosa", clientCN(req))
	})
}

// ---- git integration tests (requires git in PATH) ----

func requireGit(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH; skipping integration test")
	}
	return gitPath
}

// makeBareRepo creates a temporary bare git repo at townRoot/<rig>/.repo.git
// and returns the rig name.
func makeBareRepo(t *testing.T, gitPath, townRoot string) string {
	t.Helper()
	rig := "testrip"
	repoPath := filepath.Join(townRoot, rig, ".repo.git")
	require.NoError(t, os.MkdirAll(repoPath, 0700))
	cmd := exec.Command(gitPath, "init", "--bare", repoPath)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+t.TempDir(), // avoid polluting real home
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init --bare: %s", out)
	return rig
}

func TestHandleInfoRefsIntegration(t *testing.T) {
	gitPath := requireGit(t)
	srv, townRoot := newGitServer(t)
	rig := makeBareRepo(t, gitPath, townRoot)

	t.Run("git-upload-pack info/refs returns 200 with correct Content-Type", func(t *testing.T) {
		req := httptest.NewRequest("GET",
			"/v1/git/"+rig+"/info/refs?service=git-upload-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t,
			"application/x-git-upload-pack-advertisement",
			rec.Header().Get("Content-Type"))
		// Smart HTTP response must start with the pkt-line service header.
		body := rec.Body.String()
		assert.Contains(t, body, "# service=git-upload-pack",
			"response should contain pkt-line service header")
	})

	t.Run("git-receive-pack info/refs returns 200 with correct Content-Type", func(t *testing.T) {
		req := httptest.NewRequest("GET",
			"/v1/git/"+rig+"/info/refs?service=git-receive-pack", nil)
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t,
			"application/x-git-receive-pack-advertisement",
			rec.Header().Get("Content-Type"))
		body := rec.Body.String()
		assert.Contains(t, body, "# service=git-receive-pack")
	})
}

func TestHandleUploadPackIntegration(t *testing.T) {
	gitPath := requireGit(t)
	srv, townRoot := newGitServer(t)
	rig := makeBareRepo(t, gitPath, townRoot)

	t.Run("POST git-upload-pack returns 200", func(t *testing.T) {
		// Send a flush-only body; git-upload-pack will return its capabilities.
		req := httptest.NewRequest("POST",
			"/v1/git/"+rig+"/git-upload-pack",
			bytes.NewReader([]byte("0000")))
		req.Header.Set("Content-Type", "application/x-git-upload-pack-request")
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)

		// Status 200 is set before git runs; even if git exits non-zero, status is 200.
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/x-git-upload-pack-result",
			rec.Header().Get("Content-Type"))
	})
}

// ---- missing repo pre-flight ----

func TestHandleGitMissingRepo(t *testing.T) {
	t.Run("info/refs returns 404 for missing repo", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("GET",
			"/v1/git/missingrig/info/refs?service=git-upload-pack", nil)
		w := httptest.NewRecorder()
		srv.handleGit(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
		// Must NOT contain a pkt-line service header — that would mean headers
		// were committed before the 404 check.
		assert.NotContains(t, w.Body.String(), "# service=")
	})

	t.Run("git-upload-pack returns 404 for missing repo", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("POST",
			"/v1/git/missingrig/git-upload-pack",
			bytes.NewReader([]byte("0000")))
		w := httptest.NewRecorder()
		srv.handleGit(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("git-receive-pack returns 404 for missing repo", func(t *testing.T) {
		srv, _ := newGitServer(t)
		req := httptest.NewRequest("POST",
			"/v1/git/missingrig/git-receive-pack",
			bytes.NewReader(receivePackBody("refs/heads/polecat/furiosa-abc")))
		w := httptest.NewRecorder()
		srv.handleGit(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ---- git audit logging ----

// newGitServerWithLog creates a Server backed by a log-capturing handler.
func newGitServerWithLog(t *testing.T) (*Server, string, *logCapture) {
	t.Helper()
	lc := &logCapture{}
	townRoot := t.TempDir()
	srv, err := New(Config{TownRoot: townRoot, Logger: slog.New(lc)}, nil)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(townRoot, "testrip", ".repo.git"), 0700))
	return srv, townRoot, lc
}

// fakeGitRequest builds an httptest.Request with a TLS peer cert CN set.
func fakeGitRequest(method, path string, body []byte, cn string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if cn != "" {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Subject: pkix.Name{CommonName: cn}},
			},
		}
	}
	return req
}

// TestHandleGitAuditLog verifies that git operations produce structured audit log records.
func TestHandleGitAuditLog(t *testing.T) {
	t.Run("fetch info/refs emits INFO with identity and rig", func(t *testing.T) {
		srv, _, lc := newGitServerWithLog(t)
		req := fakeGitRequest("GET", "/v1/git/testrip/info/refs?service=git-upload-pack",
			nil, "gt-gastown-furiosa")
		srv.handleGit(httptest.NewRecorder(), req)

		e, ok := lc.findEntry(slog.LevelInfo, "git info/refs")
		require.True(t, ok, "expected INFO 'git info/refs' log record")
		assert.Equal(t, "gastown/furiosa", e.attrs["identity"])
		assert.Equal(t, "testrip", e.attrs["rig"])
		assert.Equal(t, "fetch", e.attrs["op"])
	})

	t.Run("push info/refs emits INFO with op=push", func(t *testing.T) {
		srv, _, lc := newGitServerWithLog(t)
		req := fakeGitRequest("GET", "/v1/git/testrip/info/refs?service=git-receive-pack",
			nil, "gt-gastown-furiosa")
		srv.handleGit(httptest.NewRecorder(), req)

		e, ok := lc.findEntry(slog.LevelInfo, "git info/refs")
		require.True(t, ok, "expected INFO 'git info/refs' log record")
		assert.Equal(t, "push", e.attrs["op"])
	})

	t.Run("push denied emits WARN with identity rig and refs", func(t *testing.T) {
		srv, _, lc := newGitServerWithLog(t)
		body := receivePackBody("refs/heads/main") // not allowed
		req := fakeGitRequest("POST", "/v1/git/testrip/git-receive-pack",
			body, "gt-gastown-furiosa")
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		e, ok := lc.findEntry(slog.LevelWarn, "git push denied")
		require.True(t, ok, "expected WARN 'git push denied' log record")
		assert.Equal(t, "gastown/furiosa", e.attrs["identity"])
		assert.Equal(t, "testrip", e.attrs["rig"])
		assert.Contains(t, e.attrs["refs"], "refs/heads/main")
	})

	t.Run("push denied with missing CN emits WARN with empty identity", func(t *testing.T) {
		srv, _, lc := newGitServerWithLog(t)
		body := receivePackBody("refs/heads/polecat/furiosa-abc")
		req := fakeGitRequest("POST", "/v1/git/testrip/git-receive-pack", body, "")
		rec := httptest.NewRecorder()
		srv.handleGit(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		_, ok := lc.findEntry(slog.LevelWarn, "git push denied")
		assert.True(t, ok, "expected WARN 'git push denied' log record")
	})
}

// TestHandleGitAuditLogIntegration tests audit log records for successful pack operations.
func TestHandleGitAuditLogIntegration(t *testing.T) {
	gitPath := requireGit(t)

	t.Run("fetch emits INFO git fetch record", func(t *testing.T) {
		lc := &logCapture{}
		townRoot := t.TempDir()
		srv, err := New(Config{TownRoot: townRoot, Logger: slog.New(lc)}, nil)
		require.NoError(t, err)
		makeBareRepo(t, gitPath, townRoot)

		req := fakeGitRequest("POST", "/v1/git/testrip/git-upload-pack",
			[]byte("0000"), "gt-gastown-furiosa")
		req.Header.Set("Content-Type", "application/x-git-upload-pack-request")
		srv.handleGit(httptest.NewRecorder(), req)

		e, ok := lc.findEntry(slog.LevelInfo, "git fetch")
		require.True(t, ok, "expected INFO 'git fetch' log record")
		assert.Equal(t, "gastown/furiosa", e.attrs["identity"])
		assert.Equal(t, "testrip", e.attrs["rig"])
	})
}

// TestHandleReceivePackIntegration performs a full end-to-end mTLS git push
// through a live proxy server, verifying branch authorization with a real git binary.
//
// The test issues a polecat cert (CN "gt-gastown-raider" → polecat name "raider"),
// starts the proxy with mTLS enabled, creates a local repo with a commit, then:
//   - Asserts that a push to refs/heads/polecat/raider-* (allowed) succeeds.
//   - Asserts that a push to refs/heads/main (disallowed) is rejected.
func TestHandleReceivePackIntegration(t *testing.T) {
	gitPath := requireGit(t)

	// Generate the CA and issue a polecat client cert.
	ca, err := GenerateCA(t.TempDir())
	require.NoError(t, err)

	// "raider" is the polecat name extracted from "gt-gastown-raider" by polecatName().
	// Allowed refs are refs/heads/polecat/raider-*.
	const polecatCN = "gt-gastown-raider"
	clientCertPEM, clientKeyPEM, err := ca.IssuePolecat(polecatCN, time.Hour)
	require.NoError(t, err)

	// Write cert/key/CA to temp files so git can load them via http.ssl* config.
	tmpCerts := t.TempDir()
	certFile := filepath.Join(tmpCerts, "client.crt")
	keyFile := filepath.Join(tmpCerts, "client.key")
	caFile := filepath.Join(tmpCerts, "ca.crt")
	require.NoError(t, os.WriteFile(certFile, clientCertPEM, 0600))
	require.NoError(t, os.WriteFile(keyFile, clientKeyPEM, 0600))
	require.NoError(t, os.WriteFile(caFile, ca.CertPEM, 0644))

	// Start the mTLS proxy server.
	townRoot := t.TempDir()
	srv, err := New(Config{
		ListenAddr: "127.0.0.1:0",
		TownRoot:   townRoot,
		Logger:     discardLogger(),
	}, ca)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { srv.Start(ctx) }() //nolint:errcheck

	var addr string
	require.Eventually(t, func() bool {
		if a := srv.Addr(); a != nil {
			addr = a.String()
			return true
		}
		return false
	}, 5*time.Second, 10*time.Millisecond)
	waitForServer(t, addr, 5*time.Second)

	// Create the bare repo served by the proxy.
	rig := makeBareRepo(t, gitPath, townRoot)
	repoURL := "https://" + addr + "/v1/git/" + rig

	// Build a local git repo with one commit to push.
	localRepo := t.TempDir()
	gitHome := t.TempDir()
	baseEnv := append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+gitHome,
		"GIT_TERMINAL_PROMPT=0", // suppress any interactive credential prompts
		"GIT_AUTHOR_NAME=Test Polecat",
		"GIT_AUTHOR_EMAIL=test@gt.local",
		"GIT_COMMITTER_NAME=Test Polecat",
		"GIT_COMMITTER_EMAIL=test@gt.local",
	)

	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = dir
		cmd.Env = baseEnv
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}

	runGit(localRepo, "init")
	require.NoError(t, os.WriteFile(filepath.Join(localRepo, "README"), []byte("hello"), 0644))
	runGit(localRepo, "add", "README")
	runGit(localRepo, "commit", "-m", "initial")

	// pushRef runs git push with mTLS client cert/key/CA configured via -c flags.
	pushRef := func(refspec string) ([]byte, error) {
		cmd := exec.Command(gitPath,
			"-c", "http.sslCert="+certFile,
			"-c", "http.sslKey="+keyFile,
			"-c", "http.sslCAInfo="+caFile,
			"push", repoURL, refspec,
		)
		cmd.Dir = localRepo
		cmd.Env = baseEnv
		return cmd.CombinedOutput()
	}

	t.Run("push to allowed polecat ref succeeds", func(t *testing.T) {
		out, err := pushRef("HEAD:refs/heads/polecat/raider-testpush")
		assert.NoError(t, err, "push to allowed ref should succeed:\n%s", out)
	})

	t.Run("push to disallowed ref is rejected", func(t *testing.T) {
		out, err := pushRef("HEAD:refs/heads/main")
		assert.Error(t, err, "push to disallowed ref should fail, got output:\n%s", out)
	})
}

// TestHandleGitContextCancellation verifies that cancelling the request context
// kills the underlying git subprocess and causes the handler to return promptly.
// Both handleInfoRefs and handlePack use exec.CommandContext(r.Context(), ...), so
// context cancellation must propagate to the subprocess. Contrast: TestHandleExec
// covers context cancellation for the exec handler.
func TestHandleGitContextCancellation(t *testing.T) {
	// Create stub git-upload-pack and git-receive-pack binaries that sleep
	// for 10 seconds. They are intentionally slow so the test can cancel the
	// context while the subprocess is running rather than after it exits.
	scriptDir := t.TempDir()
	for _, name := range []string{"git-upload-pack", "git-receive-pack"} {
		path := filepath.Join(scriptDir, name)
		// "exec" replaces the shell with sleep so there is only one process.
		// Without exec, the shell forks sleep and sleep inherits the stdout
		// pipe; cmd.Wait() then blocks until sleep exits even after the shell
		// is killed, preventing prompt handler return on context cancellation.
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 10\n"), 0755))
	}
	// Prepend scriptDir so exec.CommandContext resolves our stubs first.
	// minimalEnv() propagates os.Getenv("PATH") to subprocesses, so the
	// stubs are also found when the subprocess itself resolves binaries.
	t.Setenv("PATH", scriptDir+":"+os.Getenv("PATH"))

	t.Run("handleInfoRefs subprocess killed on context cancel", func(t *testing.T) {
		srv, _ := newGitServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET",
			"/v1/git/testrip/info/refs?service=git-upload-pack", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			srv.handleGit(rec, req)
			close(done)
		}()

		// Give the subprocess time to start before cancelling.
		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("handleInfoRefs did not return after context cancellation")
		}
	})

	t.Run("handlePack upload-pack subprocess killed on context cancel", func(t *testing.T) {
		srv, _ := newGitServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("POST",
			"/v1/git/testrip/git-upload-pack",
			bytes.NewReader([]byte("0000")))
		req.Header.Set("Content-Type", "application/x-git-upload-pack-request")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			srv.handleGit(rec, req)
			close(done)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("handlePack (upload-pack) did not return after context cancellation")
		}
	})

	t.Run("handlePack receive-pack subprocess killed on context cancel", func(t *testing.T) {
		srv, _ := newGitServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		body := receivePackBody("refs/heads/polecat/furiosa-abc123")
		req := httptest.NewRequest("POST",
			"/v1/git/testrip/git-receive-pack",
			bytes.NewReader(body))
		req = req.WithContext(ctx)
		// Provide a valid mTLS CN so authorizeReceivePack passes before git runs.
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{Subject: pkix.Name{CommonName: "gt-testrip-furiosa"}},
			},
		}
		rec := httptest.NewRecorder()

		done := make(chan struct{})
		go func() {
			srv.handleGit(rec, req)
			close(done)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("handlePack (receive-pack) did not return after context cancellation")
		}
	})
}
