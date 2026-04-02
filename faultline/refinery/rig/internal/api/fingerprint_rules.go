package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
)

// registerFingerprintRuleRoutes adds fingerprint rule API routes.
func (h *Handler) registerFingerprintRuleRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	mux.HandleFunc("GET /api/{project_id}/fingerprint-rules", auth(projAccess(h.listFingerprintRules)))
	mux.HandleFunc("GET /api/{project_id}/fingerprint-rules/", auth(projAccess(h.listFingerprintRules)))
	mux.HandleFunc("POST /api/{project_id}/fingerprint-rules", auth(projAccess(admin(h.createFingerprintRule))))
	mux.HandleFunc("POST /api/{project_id}/fingerprint-rules/", auth(projAccess(admin(h.createFingerprintRule))))
	mux.HandleFunc("DELETE /api/{project_id}/fingerprint-rules/{rule_id}", auth(projAccess(admin(h.deleteFingerprintRule))))
	mux.HandleFunc("DELETE /api/{project_id}/fingerprint-rules/{rule_id}/", auth(projAccess(admin(h.deleteFingerprintRule))))
}

func (h *Handler) listFingerprintRules(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	rules, err := h.DB.ListFingerprintRules(r.Context(), projectID)
	if err != nil {
		h.Log.Error("list fingerprint rules", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rules == nil {
		rules = []db.FingerprintRule{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rules": rules,
	})
}

func (h *Handler) createFingerprintRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var body struct {
		Title       string `json:"title"`
		MatchType   string `json:"match_type"`
		Pattern     string `json:"pattern"`
		Fingerprint string `json:"fingerprint"`
		Priority    int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate required fields.
	if body.Title == "" || body.MatchType == "" || body.Pattern == "" || body.Fingerprint == "" {
		writeErr(w, http.StatusBadRequest, "title, match_type, pattern, and fingerprint are required")
		return
	}

	// Validate match_type.
	switch body.MatchType {
	case "exception_type", "message", "module", "tag":
		// valid
	default:
		writeErr(w, http.StatusBadRequest, "match_type must be exception_type, message, module, or tag")
		return
	}

	rule := db.FingerprintRule{
		ProjectID:   projectID,
		Title:       body.Title,
		MatchType:   body.MatchType,
		Pattern:     body.Pattern,
		Fingerprint: body.Fingerprint,
		Priority:    body.Priority,
	}

	id, err := h.DB.CreateFingerprintRule(r.Context(), rule)
	if err != nil {
		h.Log.Error("create fingerprint rule", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("fingerprint rule created", "id", id, "project_id", projectID, "title", body.Title)

	writeJSON(w, http.StatusCreated, map[string]string{
		"id": id,
	})
}

func (h *Handler) deleteFingerprintRule(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	ruleID := r.PathValue("rule_id")
	if projectID <= 0 || ruleID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.DeleteFingerprintRule(r.Context(), projectID, ruleID); err != nil {
		h.Log.Error("delete fingerprint rule", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("fingerprint rule deleted", "id", ruleID, "project_id", projectID)

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
	})
}
