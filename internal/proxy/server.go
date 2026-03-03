package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Config holds configuration for the proxy server.
type Config struct {
	ListenAddr      string
	AllowedCommands []string
	// AllowedSubcommands maps each allowed command ("gt", "bd") to the set of
	// subcommands that polecats may invoke. If a command has an entry here,
	// argv[1] must appear in its list; absent argv[1] → 403.
	// If a command has NO entry, subcommands are unrestricted for that command
	// (safe for single-subcommand tools, but not intended for gt/bd).
	AllowedSubcommands map[string][]string
	// TownRoot is the path to the Gas Town root directory (e.g. ~/gt).
	// Populated from the GT_TOWN env var or ~/gt by default.
	TownRoot string
	// Logger is the structured logger to use. nil uses slog.Default().
	Logger *slog.Logger
	// ExtraSANIPs are additional IP addresses to embed in the server cert as IP SANs.
	// Merged with auto-detected local interface IPs by Start().
	ExtraSANIPs []net.IP
	// ExtraSANHosts are additional DNS names to embed in the server cert as DNS SANs.
	// Merged with the default "gt-proxy-server" DNS SAN by Start().
	ExtraSANHosts []string
	// AdminListenAddr is the address for the local admin HTTP server (no TLS).
	// The admin server exposes management endpoints for operators running on the same host.
	// If empty, no admin server is started. Recommended: "127.0.0.1:0" or "127.0.0.1:9877".
	AdminListenAddr string
	// MaxConcurrentExec caps the number of exec subprocesses that may run
	// concurrently across all clients. 0 uses the default (32).
	MaxConcurrentExec int
	// ExecRateLimit is the sustained request rate per client (identified by
	// mTLS cert CN) in requests per second. 0 uses the default (10 req/s).
	ExecRateLimit float64
	// ExecRateBurst is the maximum burst size for the per-client rate limiter.
	// 0 uses the default (20).
	ExecRateBurst int
	// ExecTimeout is the maximum duration a single exec subprocess may run.
	// 0 uses the default (60s). Use a negative value to disable the timeout.
	ExecTimeout time.Duration
}

// Server is an mTLS HTTP proxy server.
type Server struct {
	cfg           Config
	ca            *CA
	allowed       map[string]bool
	allowedSubs   map[string]map[string]bool
	resolvedPaths map[string]string
	log           *slog.Logger
	denyList      *DenyList

	// execSem is a semaphore limiting global concurrent exec subprocesses.
	execSem chan struct{}
	// execTimeout is the per-command deadline; derived from Config.ExecTimeout.
	execTimeout time.Duration
	// rateLimiters holds a *rate.Limiter per client identity (cert CN).
	rateLimiters sync.Map
	rateLimit    rate.Limit
	rateBurst    int

	lnMu    sync.Mutex
	ln      net.Listener
	adminLn net.Listener
}

// New creates a new Server with the given config and CA.
// It logs a warning if AllowedCommands is empty, since no commands would be
// permitted — a safe default but almost certainly a misconfiguration.
// Any AllowedCommands entries containing "/" or "\" are rejected and removed.
// Returns an error if Config.TownRoot is empty or not an absolute path.
func New(cfg Config, ca *CA) (*Server, error) {
	if cfg.TownRoot == "" {
		return nil, fmt.Errorf("Config.TownRoot must be non-empty")
	}
	if !filepath.IsAbs(cfg.TownRoot) {
		return nil, fmt.Errorf("Config.TownRoot must be an absolute path, got %q", cfg.TownRoot)
	}

	l := cfg.Logger
	if l == nil {
		l = slog.Default()
	}

	allowed := make(map[string]bool, len(cfg.AllowedCommands))
	for _, cmd := range cfg.AllowedCommands {
		// Issue 12: AllowedCommands must be plain names, not paths.
		if strings.ContainsAny(cmd, `/\`) {
			l.Error("AllowedCommands entry contains path separator — ignoring", "entry", cmd)
			continue
		}
		allowed[cmd] = true
	}

	// Resolve binary paths at startup to prevent PATH hijacking after startup.
	resolvedPaths := make(map[string]string, len(allowed))
	for cmd := range allowed {
		path, err := exec.LookPath(cmd)
		if err != nil {
			l.Error("command not found in PATH — removing from allowlist", "cmd", cmd)
			delete(allowed, cmd)
			continue
		}
		resolvedPaths[cmd] = path
	}

	if len(allowed) == 0 {
		l.Warn("AllowedCommands is empty — all exec requests will be denied")
	}

	// Build subcommand allowlists from config.
	allowedSubs := make(map[string]map[string]bool, len(cfg.AllowedSubcommands))
	for cmd, subs := range cfg.AllowedSubcommands {
		m := make(map[string]bool, len(subs))
		for _, sub := range subs {
			m[sub] = true
		}
		allowedSubs[cmd] = m
	}

	maxConcurrent := cfg.MaxConcurrentExec
	if maxConcurrent <= 0 {
		maxConcurrent = 32
	}
	rl := cfg.ExecRateLimit
	if rl <= 0 {
		rl = 10.0
	}
	rb := cfg.ExecRateBurst
	if rb <= 0 {
		rb = 20
	}
	et := cfg.ExecTimeout
	if et == 0 {
		et = 60 * time.Second
	}

	return &Server{
		cfg:           cfg,
		ca:            ca,
		allowed:       allowed,
		allowedSubs:   allowedSubs,
		resolvedPaths: resolvedPaths,
		log:           l,
		denyList:      NewDenyList(),
		execSem:       make(chan struct{}, maxConcurrent),
		execTimeout:   et,
		rateLimit:     rate.Limit(rl),
		rateBurst:     rb,
	}, nil
}

// Addr returns the address the server is listening on.
// Valid only after Start() has progressed past the listen call (i.e. after
// the first request is handled, or after waitForServer returns in tests).
func (s *Server) Addr() net.Addr {
	s.lnMu.Lock()
	defer s.lnMu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// AdminAddr returns the address the admin server is listening on.
// Returns nil if no admin server was configured or if Start() has not yet bound
// the admin listener.
func (s *Server) AdminAddr() net.Addr {
	s.lnMu.Lock()
	defer s.lnMu.Unlock()
	if s.adminLn == nil {
		return nil
	}
	return s.adminLn.Addr()
}

// DenyCert adds a certificate serial number to the server's deny list.
// Any active or future TLS connection presenting a cert with this serial will be
// rejected at the TLS handshake. This method is safe for concurrent use.
func (s *Server) DenyCert(serial *big.Int) {
	s.denyList.Deny(serial)
}

// Start begins listening and serving. Blocks until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	pool := x509.NewCertPool()
	pool.AddCert(s.ca.Cert)

	dl := s.denyList
	tlsCfg := &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		MinVersion: tls.VersionTLS13,
		// VerifyPeerCertificate is called after the standard chain verification
		// passes. We use it to check the leaf cert's serial against the deny list,
		// so that a revoked certificate is rejected at the TLS handshake before any
		// HTTP data is processed.
		VerifyPeerCertificate: func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
				leaf := verifiedChains[0][0]
				if dl.IsDenied(leaf.SerialNumber) {
					return fmt.Errorf("certificate serial %s has been revoked", leaf.SerialNumber.Text(16))
				}
			}
			return nil
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/exec", s.handleExec)
	mux.HandleFunc("/v1/git/", s.handleGit)

	srv := &http.Server{
		Addr:        s.cfg.ListenAddr,
		Handler:     mux,
		TLSConfig:   tlsCfg,
		ReadTimeout: 30 * time.Second,
		// WriteTimeout is generous to accommodate git push/fetch streams.
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	// Generate a server cert from our CA for TLS, including IP SANs so that clients
	// connecting by IP (e.g. containers reaching the proxy at 172.17.0.1) can verify
	// the cert without an explicit ServerName override. ExtraSANIPs are appended to
	// the auto-detected local interface IPs to support NAT/VPN/external addresses.
	//
	// Note: external NAT IPs (the IP shown by "curl ifconfig.me") cannot be
	// auto-detected because they are assigned to the router, not to any local
	// interface. Operators must declare them explicitly via ExtraSANIPs.
	ips := append(serverListenIPs(s.cfg.ListenAddr), s.cfg.ExtraSANIPs...)
	certPEM, keyPEM, err := s.ca.IssueServer("gt-proxy-server", ips, s.cfg.ExtraSANHosts, 365*24*time.Hour)
	if err != nil {
		return fmt.Errorf("issue server cert: %w", err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("load server cert: %w", err)
	}
	tlsCfg.Certificates = []tls.Certificate{tlsCert}

	// Issue 11: Use net.Listen + ServeTLS so we can expose the bound address via Addr().
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.lnMu.Lock()
	s.ln = ln
	s.lnMu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("gt-proxy-server: listening", "addr", ln.Addr(), "tls", "mTLS")
		if err := srv.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Start the local admin HTTP server if configured. The admin server does not
	// use TLS because it is intended only for same-host operator tools (witness,
	// mayor). Binding to 127.0.0.1 keeps it off-network; any process on the same
	// host can reach it, which is the intended access model.
	var adminSrv *http.Server
	if s.cfg.AdminListenAddr != "" {
		adminMux := http.NewServeMux()
		adminMux.HandleFunc("/v1/admin/deny-cert", s.handleDenyCert)
		adminMux.HandleFunc("/v1/admin/issue-cert", s.handleIssueCert)

		adminSrv = &http.Server{
			Addr:         s.cfg.AdminListenAddr,
			Handler:      adminMux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		adminLn, err := net.Listen("tcp", s.cfg.AdminListenAddr)
		if err != nil {
			_ = srv.Shutdown(context.Background())
			return fmt.Errorf("admin listen: %w", err)
		}
		s.lnMu.Lock()
		s.adminLn = adminLn
		s.lnMu.Unlock()

		s.log.Info("gt-proxy-server: admin listening", "addr", adminLn.Addr())
		go func() {
			if err := adminSrv.Serve(adminLn); err != nil && err != http.ErrServerClosed {
				s.log.Error("admin server error", "err", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		// Issue 5: Give shutdown a reasonable deadline to drain in-flight requests.
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if adminSrv != nil {
			_ = adminSrv.Shutdown(shutCtx)
		}
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// serverListenIPs returns the IP addresses that should be included as IP SANs in the
// server certificate. It parses the host portion of listenAddr and:
//   - If it is a specific non-loopback IP, returns [that IP, 127.0.0.1, ::1].
//   - If it is 0.0.0.0 or :: (unspecified), enumerates all non-loopback, non-link-local
//     IPv4 and IPv6 interface addresses and prepends 127.0.0.1 and ::1.
//   - Returns [127.0.0.1, ::1] at minimum on any parse or enumeration error.
func serverListenIPs(listenAddr string) []net.IP {
	loopback4 := net.ParseIP("127.0.0.1")
	loopback6 := net.ParseIP("::1")

	host, _, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return []net.IP{loopback4, loopback6}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// hostname or empty — just use loopback
		return []net.IP{loopback4, loopback6}
	}

	if ip.IsUnspecified() {
		// 0.0.0.0 or :: — include both loopbacks plus all non-loopback, non-link-local
		// interface addresses (both IPv4 and IPv6).
		ips := []net.IP{loopback4, loopback6}
		ifaces, err := net.Interfaces()
		if err != nil {
			return ips
		}
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ifaceIP net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ifaceIP = v.IP
				case *net.IPAddr:
					ifaceIP = v.IP
				}
				if ifaceIP == nil || ifaceIP.IsLoopback() || ifaceIP.IsLinkLocalUnicast() {
					continue
				}
				if ip4 := ifaceIP.To4(); ip4 != nil {
					ips = append(ips, ip4)
				} else {
					ips = append(ips, ifaceIP)
				}
			}
		}
		return ips
	}

	if ip.IsLoopback() {
		return []net.IP{loopback4, loopback6}
	}
	// Specific non-loopback IP: include both that IP and loopbacks for local connections.
	return []net.IP{ip, loopback4, loopback6}
}

// issueCertRequest is the JSON body for POST /v1/admin/issue-cert.
type issueCertRequest struct {
	// Rig is the rig name (e.g. "MyRig").
	Rig string `json:"rig"`
	// Name is the polecat name (e.g. "rust").
	Name string `json:"name"`
	// TTL is the certificate validity duration (e.g. "720h"). Defaults to 720h (30 days).
	TTL string `json:"ttl"`
}

// issueCertResponse is the JSON response for POST /v1/admin/issue-cert.
type issueCertResponse struct {
	CN        string `json:"cn"`
	Cert      string `json:"cert"`
	Key       string `json:"key"`
	CA        string `json:"ca"`
	Serial    string `json:"serial"`
	ExpiresAt string `json:"expires_at"`
}

// handleIssueCert handles POST /v1/admin/issue-cert on the local admin server.
// It issues a new polecat client certificate signed by the server's CA.
func (s *Server) handleIssueCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	var req issueCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Rig == "" {
		http.Error(w, "bad request: rig is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "bad request: name is required", http.StatusBadRequest)
		return
	}

	ttl := 720 * time.Hour // 30 days
	if req.TTL != "" {
		parsed, err := time.ParseDuration(req.TTL)
		if err != nil {
			http.Error(w, "bad request: invalid ttl: "+err.Error(), http.StatusBadRequest)
			return
		}
		if parsed <= 0 {
			http.Error(w, "bad request: ttl must be positive", http.StatusBadRequest)
			return
		}
		ttl = parsed
	}

	cn := "gt-" + req.Rig + "-" + req.Name
	certPEM, keyPEM, err := s.ca.IssuePolecat(cn, ttl)
	if err != nil {
		http.Error(w, "failed to issue certificate: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse the cert to extract serial and expiry for the response.
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		http.Error(w, "internal error: failed to decode issued certificate PEM", http.StatusInternalServerError)
		return
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.log.Info("cert issued via admin API", "cn", cn, "serial", leaf.SerialNumber.Text(16))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(issueCertResponse{
		CN:        cn,
		Cert:      string(certPEM),
		Key:       string(keyPEM),
		CA:        string(s.ca.CertPEM),
		Serial:    leaf.SerialNumber.Text(16),
		ExpiresAt: leaf.NotAfter.UTC().Format(time.RFC3339),
	})
}

// denyCertRequest is the JSON body for POST /v1/admin/deny-cert.
type denyCertRequest struct {
	// Serial is the certificate serial number in lowercase hexadecimal (no "0x" prefix).
	Serial string `json:"serial"`
}

// handleDenyCert handles POST /v1/admin/deny-cert on the local admin server.
// It adds the given certificate serial number to the server's deny list so that
// any subsequent TLS handshake presenting that certificate is rejected.
//
// The admin server is local-only (bound to 127.0.0.1), so no additional
// authentication is required beyond having local access to the host.
func (s *Server) handleDenyCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<10) // 1 KiB is ample for a serial
	var req denyCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Serial == "" {
		http.Error(w, "bad request: serial is required", http.StatusBadRequest)
		return
	}

	serial := new(big.Int)
	if _, ok := serial.SetString(req.Serial, 16); !ok {
		http.Error(w, "bad request: serial must be lowercase hex (no 0x prefix)", http.StatusBadRequest)
		return
	}

	s.denyList.Deny(serial)
	s.log.Info("cert revoked via admin API", "serial", req.Serial)
	w.WriteHeader(http.StatusNoContent)
}

// minimalEnv returns a minimal environment for git and gt/bd subprocesses,
// containing only HOME and PATH to avoid leaking server credentials.
// GIT_EXEC_PATH is intentionally omitted: the git binary resolves it
// automatically from its own installation path, so passing HOME and PATH
// is sufficient for git subcommands to locate git-core helpers.
func minimalEnv() []string {
	env := []string{}
	for _, key := range []string{"HOME", "PATH"} {
		if val := os.Getenv(key); val != "" {
			env = append(env, key+"="+val)
		}
	}
	return env
}
