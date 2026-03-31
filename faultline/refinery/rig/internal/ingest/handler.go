package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/outdoorsea/faultline/internal/db"
)

// EventHook is called after an event is successfully processed.
// Used by the Gas Town bridge to check bead creation thresholds.
type EventHook func(ctx context.Context, projectID int64, groupID, title, culprit, level, platform string)

// Handler handles Sentry SDK ingest requests.
type Handler struct {
	DB        *db.DB
	Auth      *ProjectAuth
	Log       *slog.Logger
	OnEvent   EventHook // optional callback after event processing
	ScrubPII  bool      // enable PII scrubbing on event payloads
}

// HandleEnvelope handles POST /api/{project_id}/envelope/
func (h *Handler) HandleEnvelope(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.authenticateRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	hdr, items, err := ParseEnvelope(r)
	if err != nil {
		h.Log.Warn("envelope parse error", "err", err)
		http.Error(w, "invalid envelope", http.StatusBadRequest)
		return
	}

	eventID := hdr.EventID
	if eventID == "" {
		eventID = uuid.New().String()
	}
	eventID = normalizeEventID(eventID)

	processed := 0
	for _, item := range items {
		switch item.Type {
		case "event", "transaction", "error":
			if err := h.processEvent(r.Context(), projectID, eventID, item.Payload); err != nil {
				h.Log.Error("process event", "err", err, "event_id", eventID)
			} else {
				processed++
			}
		case "session":
			if err := h.processSession(r.Context(), projectID, item.Payload); err != nil {
				h.Log.Error("process session", "err", err)
			} else {
				processed++
			}
		case "sessions":
			if err := h.processSessionAggregate(r.Context(), projectID, item.Payload); err != nil {
				h.Log.Error("process sessions aggregate", "err", err)
			} else {
				processed++
			}
		default:
			// Accept and silently drop: client_report, attachment, profile, replay, etc.
		}
	}

	h.Log.Info("envelope ingested", "event_id", eventID, "project", projectID, "items", len(items), "processed", processed)
	writeJSON(w, http.StatusOK, map[string]string{"id": eventID})
}

// HandleStore handles POST /api/{project_id}/store/
func (h *Handler) HandleStore(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.authenticateRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxItemSize))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if !json.Valid(body) {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var partial struct {
		EventID string `json:"event_id"`
	}
	json.Unmarshal(body, &partial)
	eventID := partial.EventID
	if eventID == "" {
		eventID = uuid.New().String()
	}
	eventID = normalizeEventID(eventID)

	if err := h.processEvent(r.Context(), projectID, eventID, json.RawMessage(body)); err != nil {
		h.Log.Error("process store event", "err", err, "event_id", eventID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.Log.Info("store event ingested", "event_id", eventID, "project", projectID)
	writeJSON(w, http.StatusOK, map[string]string{"id": eventID})
}

func (h *Handler) authenticateRequest(r *http.Request) (int64, error) {
	// First try header-based auth.
	pid, err := h.Auth.Authenticate(r)
	if err == nil {
		// Validate project_id in URL matches auth.
		urlPID := pathProjectID(r)
		if urlPID > 0 && urlPID != pid {
			return 0, fmt.Errorf("project ID mismatch")
		}
		return pid, nil
	}

	return 0, err
}

func (h *Handler) processEvent(ctx context.Context, projectID int64, eventID string, raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fmt.Errorf("invalid JSON payload for event %s", eventID)
	}

	// Extract event metadata.
	var meta struct {
		Timestamp   interface{} `json:"timestamp"`
		Platform    string      `json:"platform"`
		Level       string      `json:"level"`
		Message     string      `json:"message"`
		Environment string      `json:"environment"`
		Release     string      `json:"release"`
		Culprit     string      `json:"culprit"`
		Exception   *struct {
			Values []struct {
				Type string `json:"type"`
			} `json:"values"`
		} `json:"exception"`
	}
	json.Unmarshal(raw, &meta)

	ts := parseTimestamp(meta.Timestamp)
	if meta.Level == "" {
		meta.Level = "error"
	}

	// Extract exception type.
	exceptionType := ""
	if meta.Exception != nil && len(meta.Exception.Values) > 0 {
		exceptionType = meta.Exception.Values[len(meta.Exception.Values)-1].Type
	}

	// Fingerprint -> issue group.
	fingerprint := Fingerprint(raw)
	title := IssueTitle(raw)
	culprit := IssueCulprit(raw)
	if culprit == "" {
		culprit = meta.Culprit
	}

	// Upsert issue group — returns the group UUID.
	groupID, created, err := h.DB.UpsertIssueGroup(ctx, fingerprint, projectID, title, culprit, meta.Level, ts)
	if err != nil {
		return fmt.Errorf("upsert issue group: %w", err)
	}
	if created {
		h.Log.Info("new issue group", "group_id", groupID, "title", title)
	}

	// Apply PII scrubbing if enabled.
	if h.ScrubPII {
		raw = ScrubEvent(raw)
	}

	// Insert event (idempotent via UNIQUE on project_id + event_id).
	inserted, err := h.DB.InsertEvent(ctx, eventID, projectID, fingerprint, groupID, ts, meta.Platform, meta.Level, culprit, meta.Message, meta.Environment, meta.Release, exceptionType, raw)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	if !inserted {
		h.Log.Debug("duplicate event", "event_id", eventID)
	}

	// Notify Gas Town bridge if configured.
	if h.OnEvent != nil && inserted {
		h.OnEvent(ctx, projectID, groupID, title, culprit, meta.Level, meta.Platform)
	}

	return nil
}

// processSession handles a single Sentry session update item.
func (h *Handler) processSession(ctx context.Context, projectID int64, raw json.RawMessage) error {
	var s struct {
		SID        string      `json:"sid"`
		DID        string      `json:"did"`
		Status     string      `json:"status"`
		Errors     int         `json:"errors"`
		Started    string      `json:"started"`
		Duration   float64     `json:"duration"`
		Init       bool        `json:"init"`
		Attrs      *struct {
			Release     string `json:"release"`
			Environment string `json:"environment"`
			UserAgent   string `json:"user_agent"`
		} `json:"attrs"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("parse session: %w", err)
	}
	if s.SID == "" {
		s.SID = uuid.New().String()
	}
	if s.Status == "" {
		s.Status = "ok"
	}

	started := parseTimestamp(interface{}(s.Started))
	var release, env, ua string
	if s.Attrs != nil {
		release = s.Attrs.Release
		env = s.Attrs.Environment
		ua = s.Attrs.UserAgent
	}

	return h.DB.UpsertSession(ctx, s.SID, projectID, s.DID, s.Status, s.Errors, started, s.Duration, release, env, ua)
}

// processSessionAggregate handles a Sentry aggregated sessions item (Node server-mode).
// Format: {"aggregates":[{"started":"...","exited":N,"errored":N,"crashed":N},...], "attrs":{...}}
func (h *Handler) processSessionAggregate(ctx context.Context, projectID int64, raw json.RawMessage) error {
	var agg struct {
		Aggregates []struct {
			Started string `json:"started"`
			Exited  int    `json:"exited"`
			Errored int    `json:"errored"`
			Crashed int    `json:"crashed"`
		} `json:"aggregates"`
		Attrs *struct {
			Release     string `json:"release"`
			Environment string `json:"environment"`
		} `json:"attrs"`
	}
	if err := json.Unmarshal(raw, &agg); err != nil {
		return fmt.Errorf("parse session aggregate: %w", err)
	}

	var release, env string
	if agg.Attrs != nil {
		release = agg.Attrs.Release
		env = agg.Attrs.Environment
	}

	// Each aggregate bucket becomes individual session records.
	// This is lossy (we don't have individual session IDs) but preserves counts.
	for _, bucket := range agg.Aggregates {
		started := parseTimestamp(interface{}(bucket.Started))
		total := bucket.Exited + bucket.Errored + bucket.Crashed
		if total == 0 {
			continue
		}
		// Store as a synthetic session with counts in the errors field.
		sid := uuid.New().String()
		status := "ok"
		errors := bucket.Errored + bucket.Crashed
		if bucket.Crashed > 0 {
			status = "crashed"
		} else if bucket.Errored > 0 {
			status = "errored"
		}
		if err := h.DB.UpsertSession(ctx, sid, projectID, "", status, errors, started, 0, release, env, ""); err != nil {
			return err
		}
	}
	return nil
}

// pathProjectID extracts the project_id from the URL path.
func pathProjectID(r *http.Request) int64 {
	s := r.PathValue("project_id")
	if s == "" {
		return 0
	}
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

func parseTimestamp(v interface{}) time.Time {
	switch t := v.(type) {
	case float64:
		sec := int64(t)
		nsec := int64((t - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02T15:04:05.000000",
		} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Now().UTC()
}

func normalizeEventID(id string) string {
	// Sentry event IDs are 32 hex chars (no dashes). Normalize to UUID format.
	id = strings.ReplaceAll(id, "-", "")
	if len(id) == 32 {
		return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	}
	return id
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
