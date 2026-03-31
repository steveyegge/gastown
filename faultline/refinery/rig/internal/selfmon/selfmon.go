// Package selfmon implements faultline self-monitoring: the error tracker
// reports its own errors to itself via its Sentry-compatible ingest API.
//
// It provides an slog.Handler wrapper that intercepts error-level log messages
// and posts them as Sentry events to the local /api/{project}/store/ endpoint.
// A guard flag prevents infinite recursion (self-report failures don't trigger
// further self-reports).
package selfmon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Config holds self-monitoring configuration.
type Config struct {
	// Endpoint is the full URL to post events to, e.g. "http://localhost:8080/api/0/store/"
	Endpoint string
	// SentryKey is the DSN public key for auth.
	SentryKey string
	// MinLevel is the minimum slog level that triggers a self-report.
	// Defaults to slog.LevelError.
	MinLevel slog.Level
}

// Handler wraps an slog.Handler and reports high-severity log entries to faultline.
type Handler struct {
	inner   slog.Handler
	cfg     Config
	client  *http.Client
	sending atomic.Bool // guard against recursion
	attrs   []slog.Attr
	groups  []string
}

// NewHandler creates a self-monitoring slog handler wrapping inner.
func NewHandler(inner slog.Handler, cfg Config) *Handler {
	return &Handler{
		inner: inner,
		cfg:   cfg,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Always forward to the inner handler first.
	err := h.inner.Handle(ctx, r)

	// Self-report if this is a high-severity message and we're not already
	// inside a self-report (prevents infinite recursion).
	if r.Level >= h.cfg.MinLevel && h.cfg.Endpoint != "" {
		if h.sending.CompareAndSwap(false, true) {
			go func() {
				defer h.sending.Store(false)
				h.report(r)
			}()
		}
	}

	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		inner:  h.inner.WithAttrs(attrs),
		cfg:    h.cfg,
		client: h.client,
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		inner:  h.inner.WithGroup(name),
		cfg:    h.cfg,
		client: h.client,
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
}

// report sends a Sentry-compatible event to the local faultline instance.
func (h *Handler) report(r slog.Record) {
	event := h.buildEvent(r)
	body, err := json.Marshal(event)
	if err != nil {
		return // can't report errors about error reporting
	}

	req, err := http.NewRequest("POST", h.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sentry-Auth", fmt.Sprintf("Sentry sentry_key=%s", h.cfg.SentryKey))

	resp, err := h.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// sentryEvent is a minimal Sentry event payload.
type sentryEvent struct {
	EventID   string            `json:"event_id"`
	Timestamp float64           `json:"timestamp"`
	Platform  string            `json:"platform"`
	Level     string            `json:"level"`
	Logger    string            `json:"logger"`
	Message   string            `json:"message"`
	Tags      map[string]string `json:"tags,omitempty"`
	Extra     map[string]any    `json:"extra,omitempty"`
	Exception *sentryException  `json:"exception,omitempty"`
}

type sentryException struct {
	Values []sentryExceptionValue `json:"values"`
}

type sentryExceptionValue struct {
	Type       string          `json:"type"`
	Value      string          `json:"value"`
	Stacktrace *sentryStack   `json:"stacktrace,omitempty"`
}

type sentryStack struct {
	Frames []sentryFrame `json:"frames"`
}

type sentryFrame struct {
	Function string `json:"function"`
	Filename string `json:"filename"`
	Lineno   int    `json:"lineno"`
}

func (h *Handler) buildEvent(r slog.Record) sentryEvent {
	id := uuid.New().String()

	level := "error"
	if r.Level >= slog.LevelError+4 { // Fatal-ish
		level = "fatal"
	} else if r.Level >= slog.LevelError {
		level = "error"
	} else if r.Level >= slog.LevelWarn {
		level = "warning"
	}

	extra := make(map[string]any)
	var errVal string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "err" || a.Key == "error" {
			errVal = a.Value.String()
		}
		extra[a.Key] = a.Value.String()
		return true
	})
	for _, a := range h.attrs {
		extra[a.Key] = a.Value.String()
	}

	// Build a stack trace from the caller.
	var stack *sentryStack
	frames := collectFrames(4) // skip selfmon internals
	if len(frames) > 0 {
		stack = &sentryStack{Frames: frames}
	}

	msg := r.Message
	exType := "faultline.internal"
	exValue := msg
	if errVal != "" {
		exValue = fmt.Sprintf("%s: %s", msg, errVal)
	}

	ev := sentryEvent{
		EventID:   id,
		Timestamp: float64(r.Time.UnixNano()) / 1e9,
		Platform:  "go",
		Level:     level,
		Logger:    "faultline.selfmon",
		Message:   msg,
		Tags: map[string]string{
			"source": "selfmon",
		},
		Extra: extra,
		Exception: &sentryException{
			Values: []sentryExceptionValue{
				{
					Type:       exType,
					Value:      exValue,
					Stacktrace: stack,
				},
			},
		},
	}

	return ev
}

// collectFrames walks the call stack and returns Sentry-compatible frames.
// Frames are returned in Sentry order (outermost first).
func collectFrames(skip int) []sentryFrame {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return nil
	}
	pcs = pcs[:n]
	frames := runtime.CallersFrames(pcs)

	var result []sentryFrame
	for {
		frame, more := frames.Next()
		result = append(result, sentryFrame{
			Function: frame.Function,
			Filename: frame.File,
			Lineno:   frame.Line,
		})
		if !more {
			break
		}
	}

	// Sentry wants outermost first (reversed from Go's stack order).
	// Actually Go's runtime.CallersFrames already returns inner-to-outer,
	// and Sentry wants frames from outermost to innermost (bottom to top),
	// so we reverse.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}
