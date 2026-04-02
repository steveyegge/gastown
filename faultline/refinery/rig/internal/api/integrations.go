package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/integrations"
)

func (h *Handler) registerIntegrationRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	mux.HandleFunc("GET /api/{project_id}/integrations/", auth(projAccess(h.listIntegrations)))
	mux.HandleFunc("POST /api/{project_id}/integrations/", auth(projAccess(admin(h.createIntegration))))
	mux.HandleFunc("GET /api/{project_id}/integrations/{integration_id}/", auth(projAccess(h.getIntegration)))
	mux.HandleFunc("PUT /api/{project_id}/integrations/{integration_id}/", auth(projAccess(admin(h.updateIntegration))))
	mux.HandleFunc("DELETE /api/{project_id}/integrations/{integration_id}/", auth(projAccess(admin(h.deleteIntegration))))

	// Without trailing slash.
	mux.HandleFunc("GET /api/{project_id}/integrations", auth(projAccess(h.listIntegrations)))
	mux.HandleFunc("POST /api/{project_id}/integrations", auth(projAccess(admin(h.createIntegration))))
	mux.HandleFunc("GET /api/{project_id}/integrations/{integration_id}", auth(projAccess(h.getIntegration)))
	mux.HandleFunc("PUT /api/{project_id}/integrations/{integration_id}", auth(projAccess(admin(h.updateIntegration))))
	mux.HandleFunc("DELETE /api/{project_id}/integrations/{integration_id}", auth(projAccess(admin(h.deleteIntegration))))
}

func (h *Handler) listIntegrations(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	configs, err := h.DB.ListIntegrations(r.Context(), projectID)
	if err != nil {
		h.Log.Error("list integrations", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if configs == nil {
		configs = []db.IntegrationConfig{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"integrations": configs})
}

func (h *Handler) getIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	integrationID := r.PathValue("integration_id")
	if projectID <= 0 || integrationID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	cfg, err := h.DB.GetIntegration(r.Context(), projectID, integrationID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "integration not found")
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handler) createIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var cfg db.IntegrationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cfg.ProjectID = projectID

	if cfg.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if cfg.IntegrationType == "" {
		writeErr(w, http.StatusBadRequest, "integration_type is required")
		return
	}
	if !integrations.IsValidType(integrations.IntegrationType(cfg.IntegrationType)) {
		writeErr(w, http.StatusBadRequest, "invalid integration_type: must be one of github_issues, pagerduty, jira, linear")
		return
	}

	if err := h.DB.InsertIntegration(r.Context(), &cfg); err != nil {
		h.Log.Error("create integration", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, cfg)
}

func (h *Handler) updateIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	integrationID := r.PathValue("integration_id")
	if projectID <= 0 || integrationID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	existing, err := h.DB.GetIntegration(r.Context(), projectID, integrationID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "integration not found")
		return
	}

	var update db.IntegrationConfig
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Preserve immutable fields.
	update.ID = existing.ID
	update.ProjectID = existing.ProjectID
	update.CreatedAt = existing.CreatedAt
	if update.IntegrationType == "" {
		update.IntegrationType = existing.IntegrationType
	}
	if update.Name == "" {
		update.Name = existing.Name
	}
	if update.Config == nil {
		update.Config = existing.Config
	}

	if update.IntegrationType != "" && !integrations.IsValidType(integrations.IntegrationType(update.IntegrationType)) {
		writeErr(w, http.StatusBadRequest, "invalid integration_type")
		return
	}

	if err := h.DB.UpdateIntegration(r.Context(), &update); err != nil {
		h.Log.Error("update integration", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, update)
}

func (h *Handler) deleteIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	integrationID := r.PathValue("integration_id")
	if projectID <= 0 || integrationID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.DeleteIntegration(r.Context(), projectID, integrationID); err != nil {
		h.Log.Error("delete integration", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
