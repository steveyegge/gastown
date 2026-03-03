// Git smart-HTTP proxy
//
// This file implements a git smart-HTTP server that bridges sandboxed containers
// to the host's bare repositories (~/gt/<rig>/.repo.git).  Containers never
// contact GitHub directly; all fetch and push traffic flows through this proxy.
//
// Protocol overview
//
//	git clone / fetch uses two round-trips over HTTP:
//	  1. GET  /v1/git/<rig>/info/refs?service=git-upload-pack
//	       Server advertises available refs.
//	  2. POST /v1/git/<rig>/git-upload-pack
//	       Client sends want/have lines; server streams the pack file.
//
//	git push uses:
//	  1. GET  /v1/git/<rig>/info/refs?service=git-receive-pack
//	       Server advertises current refs.
//	  2. POST /v1/git/<rig>/git-receive-pack
//	       Client sends the pkt-line ref-update commands + pack data.
//
//	The proxy runs the corresponding git binary (git-upload-pack or
//	git-receive-pack) against the bare repo, piping HTTP request/response
//	bodies as the subprocess stdin/stdout.  This is the same mechanism used
//	by git-http-backend.
//
// Security model
//
//   - Rig name is validated against rigNameRe (^[a-zA-Z0-9_-]+$) to prevent
//     path traversal before constructing the repo path.
//   - The repository must exist; handlers return 404 before writing any
//     response body if repoPath is missing (avoids corrupt partial responses).
//   - git-receive-pack requires a valid mTLS client cert.  The cert CN
//     (format: "gt-<rig>-<name>") is parsed to extract the polecat name.
//     Every pushed ref must match refs/heads/polecat/<name>-*; any other
//     ref is rejected with 403 before git ever sees the request body.
//   - git-upload-pack is unrestricted for any authenticated client (read-only).
//   - Subprocesses inherit only HOME and PATH from the server environment;
//     no credentials, tokens, or secrets are visible to git.
//
// Why headers are committed before the subprocess
//
//	The git smart-HTTP protocol requires the response Content-Type and the
//	pkt-line service advertisement to appear before git's output.  The HTTP
//	ResponseWriter flushes headers on the first Write call, so once the
//	pkt-line prefix is written the status is committed to 200.  The repo
//	existence pre-flight in handleGit is the safeguard: it returns a clean
//	4xx before any write occurs, eliminating the most likely failure mode.
//	Failures in the git subprocess after headers are sent are logged but
//	cannot be returned to the client as HTTP errors.
package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// rigNameRe matches valid rig names: alphanumeric, hyphens, and underscores only.
var rigNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// handleGit serves git smart-HTTP protocol for upload-pack and receive-pack.
// Routes:
//
//	GET  /v1/git/<rig>/info/refs?service=git-upload-pack
//	POST /v1/git/<rig>/git-upload-pack
//	GET  /v1/git/<rig>/info/refs?service=git-receive-pack
//	POST /v1/git/<rig>/git-receive-pack
func (s *Server) handleGit(w http.ResponseWriter, r *http.Request) {
	// Path: /v1/git/<rig>/...
	path := strings.TrimPrefix(r.URL.Path, "/v1/git/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid git path", http.StatusBadRequest)
		return
	}
	rig := parts[0]
	rest := parts[1]

	if rig == "" {
		http.Error(w, "missing rig name", http.StatusBadRequest)
		return
	}

	if !rigNameRe.MatchString(rig) {
		http.Error(w, "invalid rig name", http.StatusBadRequest)
		return
	}

	repoPath := filepath.Join(s.cfg.TownRoot, rig, ".repo.git")

	if _, err := os.Stat(repoPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, fmt.Sprintf("rig %q not found", rig), http.StatusNotFound)
			return
		}
		s.log.Error("stat repo path failed", "path", repoPath, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	switch {
	case rest == "info/refs":
		s.handleInfoRefs(w, r, repoPath, rig)
	case rest == "git-upload-pack":
		s.handlePack(w, r, repoPath, "git-upload-pack", rig, clientCN(r))
	case rest == "git-receive-pack":
		s.handlePack(w, r, repoPath, "git-receive-pack", rig, clientCN(r))
	default:
		http.Error(w, "unknown git endpoint", http.StatusNotFound)
	}
}

func clientCN(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	return r.TLS.PeerCertificates[0].Subject.CommonName
}

func (s *Server) handleInfoRefs(w http.ResponseWriter, r *http.Request, repoPath, rig string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "unsupported service", http.StatusBadRequest)
		return
	}

	identity := cnToIdentity(clientCN(r))
	op := "fetch"
	if service == "git-receive-pack" {
		op = "push"
	}
	s.log.Info("git info/refs", "identity", identity, "rig", rig, "op", op)

	// Caller (handleGit) has verified repoPath exists. Headers and the pkt-line
	// service prefix are written now because the git smart-HTTP protocol requires
	// them to precede git's output. Once written the 200 status is committed;
	// any git subprocess failure is logged but cannot be surfaced as an HTTP error.
	w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")

	// Smart HTTP info/refs pkt-line prefix.
	pktLine := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s0000", len(pktLine)+4, pktLine)

	var errBuf strings.Builder
	cmd := exec.CommandContext(r.Context(), service, "--stateless-rpc", "--advertise-refs", repoPath)
	cmd.Stdout = w
	cmd.Stderr = &errBuf
	cmd.Env = minimalEnv()
	if err := cmd.Run(); err != nil {
		s.log.Error("git info/refs failed", "service", service, "rig", rig, "err", err, "stderr", errBuf.String())
	}
}

func (s *Server) handlePack(w http.ResponseWriter, r *http.Request, repoPath, service, rig, clientCN string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identity := cnToIdentity(clientCN)

	// For receive-pack: enforce CN-scoped branch authorization.
	var refs []string
	if service == "git-receive-pack" {
		var ok bool
		ok, refs = s.authorizeReceivePack(w, r, clientCN)
		if !ok {
			s.log.Warn("git push denied", "identity", identity, "rig", rig, "refs", refs)
			return
		}
	}

	w.Header().Set("Content-Type", "application/x-"+service+"-result")
	w.Header().Set("Cache-Control", "no-cache")

	var errBuf strings.Builder
	cmd := exec.CommandContext(r.Context(), service, "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w
	cmd.Stderr = &errBuf
	cmd.Env = minimalEnv()
	if err := cmd.Run(); err != nil {
		s.log.Error("git pack failed", "service", service, "rig", rig, "err", err, "stderr", errBuf.String())
	}

	// Audit log: record who performed the operation regardless of git subprocess outcome.
	if service == "git-receive-pack" {
		s.log.Info("git push", "identity", identity, "rig", rig, "refs", refs)
	} else {
		s.log.Info("git fetch", "identity", identity, "rig", rig)
	}
}

// authorizeReceivePack checks that the push only touches refs/heads/polecat/<cn-name>-*.
// It reads the pkt-line stream to extract ref names, then rewinds the body.
// It returns (true, refs) on success, or (false, refs) on failure; refs may be
// non-nil on failure when the body was read but contained a disallowed ref.
func (s *Server) authorizeReceivePack(w http.ResponseWriter, r *http.Request, clientCN string) (bool, []string) {
	// Issue 8: Use the shared polecatName helper instead of reimplementing CN parsing.
	cnName := polecatName(clientCN)
	if cnName == "" {
		http.Error(w, "cannot determine polecat name from cert CN", http.StatusForbidden)
		return false, nil
	}

	// Read only the pkt-line ref-update section (up to the flush packet),
	// then stream the remaining pack data through to git untouched.
	// This avoids loading the entire (potentially large) pack into memory.
	const maxPktLineSection = 256 << 10 // 256 KiB: more than enough for ref lines
	var pktBuf bytes.Buffer
	limited := io.LimitReader(r.Body, maxPktLineSection)

	// Read pkt-lines until the flush packet ("0000") or limit.
	var tmp [4]byte
	for {
		_, err := io.ReadFull(limited, tmp[:])
		if err != nil {
			http.Error(w, "read pkt-line header: "+err.Error(), http.StatusBadRequest)
			return false, nil
		}
		pktBuf.Write(tmp[:])
		// Flush packet terminates the ref-update section.
		if bytes.Equal(tmp[:], []byte("0000")) {
			break
		}
		var pktLen int
		_, err = fmt.Sscanf(string(tmp[:]), "%x", &pktLen)
		if err != nil || pktLen < 4 {
			http.Error(w, "malformed pkt-line length", http.StatusBadRequest)
			return false, nil
		}
		payload := make([]byte, pktLen-4)
		_, err = io.ReadFull(limited, payload)
		if err != nil {
			http.Error(w, "read pkt-line payload: "+err.Error(), http.StatusBadRequest)
			return false, nil
		}
		pktBuf.Write(payload)
	}

	pktBytes := pktBuf.Bytes()

	// Collect refs before validation so they are available for audit logging even
	// when authorization is denied.
	refs := collectReceivePackRefs(pktBytes)

	if err := validateReceivePackRefs(pktBytes, cnName); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return false, refs
	}

	// Reconstruct body: pkt-line prefix + remaining pack data (streamed).
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(pktBytes), r.Body))
	return true, refs
}

// collectReceivePackRefs parses the git-receive-pack pkt-line stream and returns
// all ref names found, without validating whether they are authorized.
// Used solely for audit logging; validation is handled by validateReceivePackRefs.
func collectReceivePackRefs(body []byte) []string {
	var refs []string
	offset := 0
	for offset < len(body) {
		if offset+4 > len(body) {
			break
		}
		lenHex := body[offset : offset+4]
		if bytes.Equal(lenHex, []byte("0000")) {
			break
		}
		var pktLen int
		_, err := fmt.Sscanf(string(lenHex), "%x", &pktLen)
		if err != nil || pktLen < 4 {
			break
		}
		end := offset + pktLen
		if end > len(body) {
			break
		}
		line := body[offset+4 : end]
		offset = end

		line = bytes.TrimRight(line, "\n")
		if idx := bytes.IndexByte(line, 0); idx >= 0 {
			line = line[:idx]
		}
		parts := bytes.Fields(line)
		if len(parts) < 3 {
			continue
		}
		refs = append(refs, string(parts[2]))
	}
	return refs
}

// validateReceivePackRefs parses the git-receive-pack pkt-line stream and validates
// that all pushed refs are under refs/heads/polecat/<cnName>-* (prefix form only).
func validateReceivePackRefs(body []byte, cnName string) error {
	// The pkt-line wire format: each record is a 4-hex-digit length (including the
	// length field itself) followed by that many bytes of payload.  "0000" is a
	// flush packet that terminates the ref list.  Any binary pack data that follows
	// the flush packet is never read by this loop.
	allowed := "refs/heads/polecat/" + cnName + "-"
	offset := 0
	for offset < len(body) {
		// Guard: need at least 4 bytes for the length field.
		if offset+4 > len(body) {
			return fmt.Errorf("malformed pkt-line: truncated length field at offset %d", offset)
		}
		lenHex := body[offset : offset+4]
		if bytes.Equal(lenHex, []byte("0000")) {
			break // flush packet: end of ref list
		}
		var pktLen int
		_, err := fmt.Sscanf(string(lenHex), "%x", &pktLen)
		// pktLen < 4 would underflow the payload slice; treat as malformed and reject.
		if err != nil || pktLen < 4 {
			return fmt.Errorf("malformed pkt-line: invalid length field %q at offset %d", lenHex, offset)
		}
		end := offset + pktLen
		// Guard: truncated packet — length field claims more bytes than available.
		if end > len(body) {
			return fmt.Errorf("malformed pkt-line: truncated body at offset %d (need %d, have %d)", offset, pktLen, len(body)-offset)
		}
		// Payload starts after the 4-byte length prefix; always advances by pktLen
		// (even when pktLen==4, the empty payload line is skipped below).
		line := body[offset+4 : end]
		offset = end

		// Each line: "<old-sha> <new-sha> <refname>\0[capabilities]\n"
		// Strip the trailing newline, then truncate at the first NUL byte so that
		// capability strings (e.g. "\0side-band-64k") do not pollute the ref name.
		line = bytes.TrimRight(line, "\n")
		if idx := bytes.IndexByte(line, 0); idx >= 0 {
			line = line[:idx]
		}
		parts := bytes.Fields(line)
		if len(parts) < 3 {
			continue
		}
		ref := string(parts[2])

		// Only allow refs/heads/polecat/<cnName>-* (prefix form).
		// Exact-name pushes (without timestamp suffix) are not permitted.
		if !strings.HasPrefix(ref, allowed) {
			return fmt.Errorf("push to %q denied: only refs/heads/polecat/%s-* allowed", ref, cnName)
		}
	}
	return nil
}
