package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/notify"
)

func (h *Handler) registerWebhookTemplateRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	mux.HandleFunc("GET /api/{project_id}/webhook-templates/", auth(projAccess(h.listWebhookTemplates)))
	mux.HandleFunc("POST /api/{project_id}/webhook-templates/", auth(projAccess(admin(h.createWebhookTemplate))))
	mux.HandleFunc("GET /api/{project_id}/webhook-templates/{template_id}/", auth(projAccess(h.getWebhookTemplate)))
	mux.HandleFunc("PUT /api/{project_id}/webhook-templates/{template_id}/", auth(projAccess(admin(h.updateWebhookTemplate))))
	mux.HandleFunc("DELETE /api/{project_id}/webhook-templates/{template_id}/", auth(projAccess(admin(h.deleteWebhookTemplate))))
	mux.HandleFunc("POST /api/{project_id}/webhook-templates/preview/", auth(projAccess(h.previewWebhookTemplate)))
	mux.HandleFunc("GET /api/{project_id}/webhook-templates/defaults/", auth(projAccess(h.listDefaultTemplates)))

	// Without trailing slash.
	mux.HandleFunc("GET /api/{project_id}/webhook-templates", auth(projAccess(h.listWebhookTemplates)))
	mux.HandleFunc("POST /api/{project_id}/webhook-templates", auth(projAccess(admin(h.createWebhookTemplate))))
	mux.HandleFunc("GET /api/{project_id}/webhook-templates/{template_id}", auth(projAccess(h.getWebhookTemplate)))
	mux.HandleFunc("PUT /api/{project_id}/webhook-templates/{template_id}", auth(projAccess(admin(h.updateWebhookTemplate))))
	mux.HandleFunc("DELETE /api/{project_id}/webhook-templates/{template_id}", auth(projAccess(admin(h.deleteWebhookTemplate))))
	mux.HandleFunc("POST /api/{project_id}/webhook-templates/preview", auth(projAccess(h.previewWebhookTemplate)))
	mux.HandleFunc("GET /api/{project_id}/webhook-templates/defaults", auth(projAccess(h.listDefaultTemplates)))
}

func (h *Handler) listWebhookTemplates(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	templates := []db.WebhookTemplate{}
	if project.Config != nil && len(project.Config.WebhookTemplates) > 0 {
		templates = project.Config.WebhookTemplates
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": templates})
}

func (h *Handler) getWebhookTemplate(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	templateID := r.PathValue("template_id")
	if projectID <= 0 || templateID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	if project.Config != nil {
		for _, t := range project.Config.WebhookTemplates {
			if t.ID == templateID {
				writeJSON(w, http.StatusOK, t)
				return
			}
		}
	}

	writeErr(w, http.StatusNotFound, "template not found")
}

func (h *Handler) createWebhookTemplate(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var tmpl db.WebhookTemplate
	if err := json.NewDecoder(r.Body).Decode(&tmpl); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if tmpl.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if tmpl.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	if tmpl.EventType == "" {
		tmpl.EventType = "*"
	}

	// Generate ID.
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	tmpl.ID = "wt-" + hex.EncodeToString(b)
	tmpl.IsDefault = false

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	cfg := project.Config
	if cfg == nil {
		cfg = &db.ProjectConfig{}
	}
	cfg.WebhookTemplates = append(cfg.WebhookTemplates, tmpl)

	if err := h.DB.UpdateProjectConfig(r.Context(), projectID, cfg); err != nil {
		h.Log.Error("create webhook template", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

func (h *Handler) updateWebhookTemplate(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	templateID := r.PathValue("template_id")
	if projectID <= 0 || templateID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	var update db.WebhookTemplate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	cfg := project.Config
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}

	found := false
	for i, t := range cfg.WebhookTemplates {
		if t.ID == templateID {
			if update.Name != "" {
				cfg.WebhookTemplates[i].Name = update.Name
			}
			if update.EventType != "" {
				cfg.WebhookTemplates[i].EventType = update.EventType
			}
			if update.Body != "" {
				cfg.WebhookTemplates[i].Body = update.Body
			}
			found = true
			break
		}
	}

	if !found {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}

	if err := h.DB.UpdateProjectConfig(r.Context(), projectID, cfg); err != nil {
		h.Log.Error("update webhook template", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Return the updated template.
	for _, t := range cfg.WebhookTemplates {
		if t.ID == templateID {
			writeJSON(w, http.StatusOK, t)
			return
		}
	}
}

func (h *Handler) deleteWebhookTemplate(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	templateID := r.PathValue("template_id")
	if projectID <= 0 || templateID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	project, err := h.DB.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}

	cfg := project.Config
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}

	found := false
	filtered := cfg.WebhookTemplates[:0]
	for _, t := range cfg.WebhookTemplates {
		if t.ID == templateID {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}

	if !found {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}

	cfg.WebhookTemplates = filtered
	if err := h.DB.UpdateProjectConfig(r.Context(), projectID, cfg); err != nil {
		h.Log.Error("delete webhook template", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) previewWebhookTemplate(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var req struct {
		Body      string `json:"body"`
		EventType string `json:"event_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	if req.EventType == "" {
		req.EventType = "new_issue"
	}

	// Build sample event for preview.
	sampleEvent := notify.Event{
		Type:       notify.EventType(req.EventType),
		ProjectID:  projectID,
		GroupID:    "abc123def456",
		Title:      "TypeError: Cannot read property 'foo' of undefined",
		Culprit:    "app.controllers.UserController.getProfile",
		Level:      "error",
		Platform:   "javascript",
		EventCount: 42,
		BeadID:     "fl-sample",
	}

	vars := notify.TemplateVars(sampleEvent, h.BaseURL)
	rendered := notify.RenderTemplate(req.Body, vars)

	writeJSON(w, http.StatusOK, map[string]string{"rendered": rendered})
}

func (h *Handler) listDefaultTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": notify.DefaultTemplates()})
}
