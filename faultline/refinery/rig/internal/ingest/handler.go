package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/sourcemap"
)

// EventHook is called after an event is successfully processed.
// Used by the Gas Town bridge to check bead creation thresholds.
type EventHook func(ctx context.Context, projectID int64, groupID, title, culprit, level, platform string)

// Handler handles Sentry SDK ingest requests.
type Handler struct {
	DB             *db.DB
	Auth           *ProjectAuth
	Log            *slog.Logger
	OnEvent        EventHook        // optional callback after event processing
	ScrubPII       bool             // enable PII scrubbing on event payloads
	AutoRegister   bool             // auto-create projects on first event (local mode)
	SourceMapStore *sourcemap.Store // optional source map store for symbolication

	rulesMu    sync.RWMutex
	rules      map[int64][]db.FingerprintRule // cached per project
	registerMu sync.Mutex                     // serializes auto-registration
}

// RefreshRules reloads fingerprint rules from the database for a given project.
// If projectID is 0, it clears all cached rules (call LoadRules per-project on next event).
func (h *Handler) RefreshRules(ctx context.Context, projectID int64) error {
	rules, err := h.DB.ListFingerprintRules(ctx, projectID)
	if err != nil {
		return fmt.Errorf("refresh fingerprint rules: %w", err)
	}
	h.rulesMu.Lock()
	defer h.rulesMu.Unlock()
	if h.rules == nil {
		h.rules = make(map[int64][]db.FingerprintRule)
	}
	h.rules[projectID] = rules
	return nil
}

// getRules returns cached fingerprint rules for a project, loading from DB if needed.
func (h *Handler) getRules(ctx context.Context, projectID int64) []db.FingerprintRule {
	h.rulesMu.RLock()
	if h.rules != nil {
		if r, ok := h.rules[projectID]; ok {
			h.rulesMu.RUnlock()
			return r
		}
	}
	h.rulesMu.RUnlock()

	// Load from DB on cache miss.
	rules, err := h.DB.ListFingerprintRules(ctx, projectID)
	if err != nil {
		h.Log.Warn("failed to load fingerprint rules", "project_id", projectID, "err", err)
		return nil
	}
	h.rulesMu.Lock()
	if h.rules == nil {
		h.rules = make(map[int64][]db.FingerprintRule)
	}
	h.rules[projectID] = rules
	h.rulesMu.Unlock()
	return rules
}

// InvalidateRules clears cached rules for a project so they are reloaded on next use.
func (h *Handler) InvalidateRules(projectID int64) {
	h.rulesMu.Lock()
	defer h.rulesMu.Unlock()
	delete(h.rules, projectID)
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
		case "event", "error":
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
			// Accept and silently drop everything else:
			// transaction, profile, replay, client_report, attachment,
			// check_in, statsd, metric_buckets, span, etc.
			h.Log.Debug("envelope item ignored", "type", item.Type, "project", projectID)
		}
	}

	// Any SDK traffic counts as a heartbeat (project is alive).
	_ = h.DB.RecordHeartbeat(r.Context(), projectID)

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
	_ = json.Unmarshal(body, &partial)
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

	_ = h.DB.RecordHeartbeat(r.Context(), projectID)
	h.Log.Info("store event ingested", "event_id", eventID, "project", projectID)
	writeJSON(w, http.StatusOK, map[string]string{"id": eventID})
}

// IngestRaw processes a raw Sentry envelope payload for a given project.
// Used by the relay poller to feed pulled envelopes through the normal pipeline.
// The payload may be gzip-compressed (when the SDK sent Content-Encoding: gzip
// and the relay stored the raw body without decompressing).
func (h *Handler) IngestRaw(ctx context.Context, projectID int64, payload []byte) error {
	// Detect and decompress gzip payloads. The gzip magic number is 0x1f 0x8b.
	if len(payload) >= 2 && payload[0] == 0x1f && payload[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("parse envelope: gzip: %w", err)
		}
		decompressed, err := io.ReadAll(gz)
		_ = gz.Close()
		if err != nil {
			return fmt.Errorf("parse envelope: gzip read: %w", err)
		}
		payload = decompressed
	}

	hdr, items, err := parseEnvelopeBytes(payload)
	if err != nil {
		return fmt.Errorf("parse envelope: %w", err)
	}

	eventID := hdr.EventID
	if eventID == "" {
		eventID = uuid.New().String()
	}
	eventID = normalizeEventID(eventID)

	processed := 0
	for _, item := range items {
		switch item.Type {
		case "event", "error":
			if err := h.processEvent(ctx, projectID, eventID, item.Payload); err != nil {
				h.Log.Error("process event (relay)", "err", err, "event_id", eventID)
			} else {
				processed++
			}
		case "session":
			if err := h.processSession(ctx, projectID, item.Payload); err != nil {
				h.Log.Error("process session (relay)", "err", err)
			} else {
				processed++
			}
		case "sessions":
			if err := h.processSessionAggregate(ctx, projectID, item.Payload); err != nil {
				h.Log.Error("process sessions aggregate (relay)", "err", err)
			} else {
				processed++
			}
		default:
			// Silently accept everything else (transactions, profiles, replays, etc.)
		}
	}

	h.Log.Info("relay envelope ingested", "event_id", eventID, "project", projectID, "items", len(items), "processed", processed)
	return nil
}

// ProcessCIEvent ingests a CI failure event through the normal pipeline.
// Used by the CI webhook handler to convert CI failures to faultline issues.
func (h *Handler) ProcessCIEvent(ctx context.Context, projectID int64, eventID string, raw json.RawMessage) error {
	return h.processEvent(ctx, projectID, eventID, raw)
}

// HandleHeartbeat accepts a lightweight ping from SDKs to register as active.
// Updates the project's last_heartbeat timestamp without creating any events.
// If the body contains JSON with a "url" field, the project's config URL is updated.
func (h *Handler) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.authenticateRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err := h.DB.RecordHeartbeat(r.Context(), projectID); err != nil {
		h.Log.Error("heartbeat", "err", err, "project", projectID)
	}

	// Parse optional metadata from body (URL, etc.).
	if r.Body != nil && r.ContentLength != 0 {
		var meta struct {
			URL string `json:"url"`
		}
		if json.NewDecoder(r.Body).Decode(&meta) == nil && meta.URL != "" {
			h.updateProjectURL(r.Context(), projectID, meta.URL)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// updateProjectURL sets the URL field on a project's config, merging with existing config.
func (h *Handler) updateProjectURL(ctx context.Context, projectID int64, url string) {
	project, err := h.DB.GetProject(ctx, projectID)
	if err != nil {
		return
	}
	cfg := project.Config
	if cfg == nil {
		cfg = &db.ProjectConfig{}
	}
	if cfg.URL == url {
		return // already set
	}
	cfg.URL = url
	if err := h.DB.UpdateProjectConfig(ctx, projectID, cfg); err != nil {
		h.Log.Error("update project URL", "err", err, "project", projectID)
	}
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

	// Auto-register: if key is unknown and auto-registration is enabled (local mode),
	// create the project on first event. The DSN key becomes the project identifier.
	if h.AutoRegister && h.DB != nil {
		key := extractKey(r)
		if key != "" {
			pid, regErr := h.autoRegisterProject(r.Context(), key)
			if regErr != nil {
				h.Log.Error("auto-register project", "err", regErr, "key", key)
				return 0, err // return original auth error
			}
			return pid, nil
		}
	}

	return 0, err
}

// autoRegisterProject creates a new project for an unknown DSN key.
// The key is used as both the public key and the basis for the project name/slug.
func (h *Handler) autoRegisterProject(ctx context.Context, key string) (int64, error) {
	h.registerMu.Lock()
	defer h.registerMu.Unlock()

	// Double-check under lock — another goroutine may have registered it.
	if pid, err := h.Auth.Authenticate2(key); err == nil {
		return pid, nil
	}

	// Derive project name from key: use the key itself, truncated for readability.
	slug := key
	if len(slug) > 32 {
		slug = slug[:32]
	}

	// Find next project ID.
	projects, err := h.DB.ListProjects(ctx)
	if err != nil {
		return 0, fmt.Errorf("auto-register: list projects: %w", err)
	}
	var maxID int64
	for _, p := range projects {
		if p.ID > maxID {
			maxID = p.ID
		}
	}
	projectID := maxID + 1

	// Create in database.
	if err := h.DB.EnsureProject(ctx, projectID, slug, slug, key); err != nil {
		return 0, fmt.Errorf("auto-register: create project: %w", err)
	}

	// Register in auth (live).
	h.Auth.Register(key, projectID, slug)

	h.Log.Info("auto-registered project", "project_id", projectID, "slug", slug, "key", key)
	return projectID, nil
}

func (h *Handler) processEvent(ctx context.Context, projectID int64, eventID string, raw json.RawMessage) error {
	// Extract event metadata.
	// Exception can be either {"values": [...]} or a raw array [...].
	var meta struct {
		Timestamp   interface{}     `json:"timestamp"`
		Platform    string          `json:"platform"`
		Level       string          `json:"level"`
		Message     string          `json:"message"`
		Environment string          `json:"environment"`
		Release     string          `json:"release"`
		Culprit     string          `json:"culprit"`
		Exception   json.RawMessage `json:"exception"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("parse event metadata: %w", err)
	}

	ts := parseTimestamp(meta.Timestamp)
	if meta.Level == "" {
		meta.Level = "error"
	}

	// Apply source map symbolication before fingerprinting so that
	// fingerprints use the original (deobfuscated) stack traces.
	if h.SourceMapStore != nil && meta.Release != "" {
		symbolicated, symErr := sourcemap.Symbolicate(h.SourceMapStore, projectID, meta.Release, raw)
		if symErr != nil {
			h.Log.Warn("symbolication failed", "err", symErr, "release", meta.Release)
		} else {
			raw = symbolicated
		}
	}

	// Extract exception type. Exception can be {"values": [...]} or bare [...].
	exceptionType := ""
	if len(meta.Exception) > 0 {
		type exVal struct {
			Type string `json:"type"`
		}
		// Try {"values": [...]} first.
		var wrapped struct {
			Values []exVal `json:"values"`
		}
		if json.Unmarshal(meta.Exception, &wrapped) == nil && len(wrapped.Values) > 0 {
			exceptionType = wrapped.Values[len(wrapped.Values)-1].Type
		} else {
			// Try bare array [...].
			var bare []exVal
			if json.Unmarshal(meta.Exception, &bare) == nil && len(bare) > 0 {
				exceptionType = bare[len(bare)-1].Type
			}
		}
	}

	// Fingerprint -> issue group (apply custom rules if any).
	rules := h.getRules(ctx, projectID)
	var fingerprint string
	if len(rules) > 0 {
		fingerprint = FingerprintWithRules(raw, rules)
	} else {
		fingerprint = Fingerprint(raw)
	}
	title := IssueTitle(raw)
	culprit := IssueCulprit(raw)
	if culprit == "" {
		culprit = meta.Culprit
	}

	// Upsert issue group — returns the group UUID.
	groupID, created, err := h.DB.UpsertIssueGroup(ctx, fingerprint, projectID, title, culprit, meta.Level, meta.Platform, ts)
	if err != nil {
		return fmt.Errorf("upsert issue group: %w", err)
	}
	if created {
		h.Log.Info("new issue group", "group_id", groupID, "title", title)
		_ = h.DB.InsertLifecycleEvent(ctx, projectID, groupID, db.LifecycleDetection, nil, nil, map[string]interface{}{
			"title":    title,
			"culprit":  culprit,
			"level":    meta.Level,
			"platform": meta.Platform,
		})
	}

	// Drop events for ignored issues — don't store, don't regress, don't notify.
	if !created {
		existing, lookupErr := h.DB.GetIssueGroup(ctx, projectID, groupID)
		if lookupErr == nil && existing.Status == "ignored" {
			return nil
		}
	}

	// Regression detection: if an existing resolved group receives a new event, mark it regressed.
	if !created {
		existing, lookupErr := h.DB.GetIssueGroup(ctx, projectID, groupID)
		if lookupErr == nil && existing.Status == "resolved" {
			if regErr := h.DB.RegressIssueGroup(ctx, projectID, groupID); regErr != nil {
				h.Log.Error("regress issue group", "err", regErr, "group_id", groupID)
			} else {
				h.Log.Warn("regression detected", "group_id", groupID, "title", title)
			}
		}
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

	// Upsert release aggregation.
	if meta.Release != "" && inserted {
		if err := h.DB.UpsertRelease(ctx, projectID, meta.Release, ts); err != nil {
			h.Log.Error("upsert release", "err", err, "release", meta.Release)
		}
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
		SID      string  `json:"sid"`
		DID      string  `json:"did"`
		Status   string  `json:"status"`
		Errors   int     `json:"errors"`
		Started  string  `json:"started"`
		Duration float64 `json:"duration"`
		Init     bool    `json:"init"`
		Attrs    *struct {
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
	_ = json.NewEncoder(w).Encode(v)
}
