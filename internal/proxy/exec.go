package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"golang.org/x/time/rate"
)

// execRequest is the body for POST /v1/exec.
type execRequest struct {
	Argv []string `json:"argv"`
}

// execResponse is the response for POST /v1/exec.
type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body to prevent a misbehaving client from exhausting memory.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	// Extract identity from client cert CN (format: gt-<rig>-<name>).
	identity := extractIdentity(r)

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Argv) == 0 {
		http.Error(w, "argv is empty", http.StatusBadRequest)
		return
	}

	// Validate argv[0] is in the allowlist.
	cmd0 := req.Argv[0]
	if !s.isAllowed(cmd0) {
		http.Error(w, fmt.Sprintf("command not allowed: %q", cmd0), http.StatusForbidden)
		return
	}

	// Validate argv[1] (subcommand) if this command has a subcommand allowlist.
	if subs, ok := s.allowedSubs[cmd0]; ok {
		if len(req.Argv) < 2 {
			http.Error(w, "subcommand required", http.StatusForbidden)
			return
		}
		sub := req.Argv[1]
		if !subs[sub] {
			http.Error(w, fmt.Sprintf("subcommand not allowed: %q %q", cmd0, sub), http.StatusForbidden)
			return
		}
	}

	// Build argv as a copy of req.Argv to avoid mutating the decoded request.
	argv := append([]string(nil), req.Argv...)
	// Use the resolved absolute binary path to prevent PATH hijacking after startup.
	if resolved, ok := s.resolvedPaths[cmd0]; ok {
		argv[0] = resolved
	}

	// Per-client rate limiting: identified by cert CN (or "unknown" if absent).
	rateKey := identity
	if rateKey == "" {
		rateKey = "unknown"
	}
	if !s.limiterFor(rateKey).Allow() {
		s.log.Warn("exec rate limit exceeded", "identity", identity)
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Global concurrency cap: reject immediately if all slots are busy.
	select {
	case s.execSem <- struct{}{}:
		defer func() { <-s.execSem }()
	default:
		s.log.Warn("exec concurrency limit exceeded", "identity", identity)
		http.Error(w, "server busy", http.StatusServiceUnavailable)
		return
	}

	execCtx := r.Context()
	if s.execTimeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, s.execTimeout)
		defer cancel()
	}
	out, errOut, exitCode := runCommand(execCtx, argv, identity)

	// Audit log (do not log full argv — it may contain tokens or secrets).
	if exitCode == 0 {
		s.log.Info("exec", "identity", identity, "cmd", cmd0,
			"sub", subForLog(req.Argv), "exit", exitCode)
	} else {
		s.log.Warn("exec failed", "identity", identity, "cmd", cmd0,
			"sub", subForLog(req.Argv), "exit", exitCode)
	}

	// The handler always returns HTTP 200 even when the subprocess exits
	// non-zero. This is intentional: the RPC call itself succeeded (the request was
	// well-formed, the command was allowed, and the subprocess ran). The subprocess's
	// outcome is reported in the JSON body via exitCode. Callers must inspect exitCode
	// rather than the HTTP status to determine whether the command succeeded.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(execResponse{
		Stdout:   out,
		Stderr:   errOut,
		ExitCode: exitCode,
	})
}

// subForLog returns argv[1] if present, otherwise "".
// Used for audit logging to capture the subcommand without logging full argv.
func subForLog(argv []string) string {
	if len(argv) >= 2 {
		return argv[1]
	}
	return ""
}

// extractIdentity parses the client cert CN "gt-<rig>-<name>" into "<rig>/<name>".
// Uses LastIndex to correctly handle hyphenated rig names (e.g. "gas-town").
func extractIdentity(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	cn := r.TLS.PeerCertificates[0].Subject.CommonName
	return cnToIdentity(cn)
}

// polecatName extracts the polecat name from a CN of the form "gt-<rig>-<name>".
// The last "-" is the rig/name separator, so hyphenated rig names are handled correctly.
// Returns "" if the CN does not match the expected format, or if rig or name is empty.
func polecatName(cn string) string {
	if !strings.HasPrefix(cn, "gt-") {
		return ""
	}
	rest := cn[3:] // strip "gt-"
	idx := strings.LastIndex(rest, "-")
	// idx <= 0: idx < 0 means no separator; idx == 0 means rig is empty.
	if idx <= 0 {
		return ""
	}
	return rest[idx+1:]
}

// cnToIdentity converts a CN of the form "gt-<rig>-<name>" to "<rig>/<name>".
// The last "-" is treated as the rig/name separator, so hyphenated rig names
// (e.g. "gas-town") are handled correctly.
func cnToIdentity(cn string) string {
	name := polecatName(cn)
	if name == "" {
		return ""
	}
	// rig is everything between "gt-" and "-<name>".
	rest := cn[3:] // strip "gt-"
	rig := rest[:len(rest)-len(name)-1]
	if rig == "" {
		return ""
	}
	return rig + "/" + name
}

// isAllowed reports whether cmd is in the allowlist.
func (s *Server) isAllowed(cmd string) bool {
	return s.allowed[cmd]
}

// limiterFor returns the rate.Limiter for the given client identity, creating
// one if it does not exist. The limiter is stored in a sync.Map so concurrent
// requests for the same identity safely share a single limiter.
//
// Note: entries are never evicted; each unique CN accumulates ~200 bytes.
// Acceptable for typical deployments (dozens of polecats); consider adding a
// periodic sweep if the server handles thousands of unique certs.
func (s *Server) limiterFor(identity string) *rate.Limiter {
	if v, ok := s.rateLimiters.Load(identity); ok {
		return v.(*rate.Limiter)
	}
	l := rate.NewLimiter(s.rateLimit, s.rateBurst)
	v, _ := s.rateLimiters.LoadOrStore(identity, l)
	return v.(*rate.Limiter)
}

func runCommand(ctx context.Context, argv []string, identity string) (stdout, stderr string, exitCode int) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	// Restrict the subprocess environment to prevent server credentials from
	// leaking into gt/bd calls. Pass identity via env var so commands can
	// optionally use it without requiring a --identity CLI flag on every command.
	env := minimalEnv()
	if identity != "" {
		env = append(env, "GT_PROXY_IDENTITY="+identity)
	}
	cmd.Env = env
	err := cmd.Run()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		} else {
			exitCode = 1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
