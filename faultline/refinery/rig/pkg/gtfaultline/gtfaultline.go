// Package gtfaultline provides a thin wrapper around sentry-go for reporting
// errors to a local faultline instance. It is designed for Gas Town rig
// dogfooding — agents report their own errors back to faultline.
package gtfaultline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
)

// DefaultDSN is the default DSN pointing at a local faultline instance.
const DefaultDSN = "http://default_key@localhost:8080/1"

// Config holds initialization options for the faultline SDK.
type Config struct {
	// DSN is the Sentry-compatible DSN. Defaults to DefaultDSN.
	DSN string
	// Release identifies the running software version.
	Release string
	// Environment identifies the deployment environment (e.g. "production").
	Environment string
	// ServerName identifies the host.
	ServerName string
	// URL is the project's web URL (e.g. "http://localhost:3000").
	// Reported to faultline so it can link directly to the running service.
	URL string
	// Debug enables verbose SDK logging.
	Debug bool
}

// Init initializes the sentry-go SDK pointing at the configured faultline
// instance. Call this once at program startup. Sends a heartbeat ping so
// faultline knows the SDK is connected (shows as "Running" on dashboard).
func Init(cfg Config) error {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = DefaultDSN
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Release:     cfg.Release,
		Environment: cfg.Environment,
		ServerName:  cfg.ServerName,
		Debug:       cfg.Debug,
	})
	if err != nil {
		return err
	}

	// Send a lightweight heartbeat ping (not an error event).
	go sendHeartbeat(dsn, cfg.URL)

	return nil
}

// sendHeartbeat pings the faultline heartbeat endpoint to register as active.
// If projectURL is non-empty, it is sent as JSON so faultline can link to the service.
func sendHeartbeat(dsn string, projectURL string) {
	// Extract base URL from DSN: http://key@host:port/project_id → http://host:port
	// DSN format: scheme://public_key@host/project_id
	parts := parseDSN(dsn)
	if parts.baseURL == "" {
		return
	}
	endpoint := fmt.Sprintf("%s/api/%s/heartbeat", parts.baseURL, parts.projectID)

	var body io.Reader
	if projectURL != "" {
		payload, _ := json.Marshal(map[string]string{"url": projectURL})
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return
	}
	req.Header.Set("X-Sentry-Auth", fmt.Sprintf("Sentry sentry_key=%s", parts.publicKey))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

type dsnParts struct {
	baseURL   string
	publicKey string
	projectID string
}

func parseDSN(dsn string) dsnParts {
	// http://key@host:port/project_id
	var p dsnParts
	idx := indexOf(dsn, "://")
	if idx < 0 {
		return p
	}
	scheme := dsn[:idx]
	rest := dsn[idx+3:]

	atIdx := indexOf(rest, "@")
	if atIdx < 0 {
		return p
	}
	p.publicKey = rest[:atIdx]
	rest = rest[atIdx+1:]

	slashIdx := lastIndexOf(rest, "/")
	if slashIdx < 0 {
		return p
	}
	p.projectID = rest[slashIdx+1:]
	p.baseURL = scheme + "://" + rest[:slashIdx]
	return p
}

func indexOf(s, sep string) int {
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, sep string) int {
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// CaptureError captures an error event and sends it to faultline.
func CaptureError(err error) {
	sentry.CaptureException(err)
}

// CaptureMessage captures a message event and sends it to faultline.
func CaptureMessage(msg string) {
	sentry.CaptureMessage(msg)
}

// RecoverAndReport is designed for use with defer:
//
//	defer gtfaultline.RecoverAndReport()
//
// It recovers from panics, reports them to faultline, then re-panics so the
// original crash behavior is preserved.
func RecoverAndReport() {
	if r := recover(); r != nil {
		var err error
		switch v := r.(type) {
		case error:
			err = v
		default:
			err = fmt.Errorf("panic: %v", v)
		}
		sentry.CaptureException(err)
		_ = sentry.Flush(2 * time.Second)
		panic(r)
	}
}

// Flush waits until the underlying transport sends any buffered events to
// faultline, blocking for at most the given timeout.
func Flush(timeout time.Duration) {
	_ = sentry.Flush(timeout)
}

// slogHandler is a slog.Handler that reports log records at or above a
// configured level to faultline, while forwarding all records to a wrapped
// handler.
type slogHandler struct {
	next     slog.Handler
	minLevel slog.Level
	attrs    []slog.Attr
	groups   []string
}

// SlogHandler returns a slog.Handler that sends Sentry events for log records
// at or above minLevel, while passing every record through to next.
func SlogHandler(next slog.Handler, minLevel slog.Level) slog.Handler {
	return &slogHandler{next: next, minLevel: minLevel}
}

func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *slogHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= h.minLevel {
		sentry.CaptureMessage(r.Message)
	}
	return h.next.Handle(ctx, r)
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &slogHandler{
		next:     h.next.WithAttrs(attrs),
		minLevel: h.minLevel,
		attrs:    append(h.attrs, attrs...),
		groups:   h.groups,
	}
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	return &slogHandler{
		next:     h.next.WithGroup(name),
		minLevel: h.minLevel,
		attrs:    h.attrs,
		groups:   append(h.groups, name),
	}
}
