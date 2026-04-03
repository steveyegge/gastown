package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
)

func (h *Handler) registerContainerRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	member := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("member", h) }
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	// List containers for a project.
	mux.HandleFunc("GET /api/{project_id}/containers/", auth(projAccess(member(h.listContainers))))
	mux.HandleFunc("GET /api/{project_id}/containers", auth(projAccess(member(h.listContainers))))

	// Container check history.
	mux.HandleFunc("GET /api/{project_id}/containers/{container_id}/checks/", auth(projAccess(member(h.listContainerChecks))))
	mux.HandleFunc("GET /api/{project_id}/containers/{container_id}/checks", auth(projAccess(member(h.listContainerChecks))))

	// Container stats (current snapshot).
	mux.HandleFunc("GET /api/{project_id}/containers/{container_id}/stats/", auth(projAccess(member(h.getContainerStats))))
	mux.HandleFunc("GET /api/{project_id}/containers/{container_id}/stats", auth(projAccess(member(h.getContainerStats))))

	// Docker threshold overrides (project-level settings).
	mux.HandleFunc("GET /api/{project_id}/settings/docker/", auth(projAccess(member(h.getDockerSettings))))
	mux.HandleFunc("GET /api/{project_id}/settings/docker", auth(projAccess(member(h.getDockerSettings))))
	mux.HandleFunc("PUT /api/{project_id}/settings/docker/", auth(projAccess(admin(h.updateDockerSettings))))
	mux.HandleFunc("PUT /api/{project_id}/settings/docker", auth(projAccess(admin(h.updateDockerSettings))))

	// Admin: list all containers across projects.
	mux.HandleFunc("GET /api/v1/system/containers/", auth(admin(h.listAllContainers)))
	mux.HandleFunc("GET /api/v1/system/containers", auth(admin(h.listAllContainers)))
}

// containerResponse is the JSON shape returned for a monitored container.
type containerResponse struct {
	ID            string  `json:"id"`
	ProjectID     *int64  `json:"project_id,omitempty"`
	ContainerID   string  `json:"container_id"`
	ContainerName string  `json:"container_name"`
	ServiceName   string  `json:"service_name,omitempty"`
	Image         string  `json:"image,omitempty"`
	Enabled       bool    `json:"enabled"`
	DiscoveredAt  string  `json:"discovered_at"`
	LastSeenAt    string  `json:"last_seen_at"`
}

func toContainerResponse(c *db.MonitoredContainer) containerResponse {
	return containerResponse{
		ID:            c.ID,
		ProjectID:     c.ProjectID,
		ContainerID:   c.ContainerID,
		ContainerName: c.ContainerName,
		ServiceName:   c.ServiceName,
		Image:         c.Image,
		Enabled:       c.Enabled,
		DiscoveredAt:  c.DiscoveredAt.Format("2006-01-02T15:04:05Z"),
		LastSeenAt:    c.LastSeenAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *Handler) listContainers(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	containers, err := h.DB.ListContainersByProject(r.Context(), projectID)
	if err != nil {
		h.Log.Error("list containers", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]containerResponse, len(containers))
	for i := range containers {
		resp[i] = toContainerResponse(&containers[i])
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"containers": resp})
}

func (h *Handler) listContainerChecks(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	containerID := r.PathValue("container_id")
	if projectID <= 0 || containerID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	// Verify the container belongs to the project.
	if _, err := h.DB.GetContainer(r.Context(), projectID, containerID); err != nil {
		writeErr(w, http.StatusNotFound, "container not found")
		return
	}

	limit := queryInt(r, "limit", 100)
	checks, err := h.DB.ListContainerChecks(r.Context(), containerID, limit)
	if err != nil {
		h.Log.Error("list container checks", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if checks == nil {
		checks = []db.ContainerCheck{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"checks": checks})
}

// containerStatsResponse combines the current monitor state with recent checks.
type containerStatsResponse struct {
	Status              string  `json:"status"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	LastCheckAt         *string `json:"last_check_at,omitempty"`
	LastTransitionAt    *string `json:"last_transition_at,omitempty"`
}

func (h *Handler) getContainerStats(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	containerID := r.PathValue("container_id")
	if projectID <= 0 || containerID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	// Verify the container belongs to the project.
	if _, err := h.DB.GetContainer(r.Context(), projectID, containerID); err != nil {
		writeErr(w, http.StatusNotFound, "container not found")
		return
	}

	state, err := h.DB.GetContainerMonitorState(r.Context(), containerID)
	if err != nil {
		// No state yet — return defaults.
		writeJSON(w, http.StatusOK, containerStatsResponse{
			Status:              "healthy",
			ConsecutiveFailures: 0,
		})
		return
	}

	resp := containerStatsResponse{
		Status:              state.Status,
		ConsecutiveFailures: state.ConsecutiveFailures,
	}
	if state.LastCheckAt != nil {
		s := state.LastCheckAt.Format("2006-01-02T15:04:05Z")
		resp.LastCheckAt = &s
	}
	if state.LastTransitionAt != nil {
		s := state.LastTransitionAt.Format("2006-01-02T15:04:05Z")
		resp.LastTransitionAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getDockerSettings(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	thresholds, err := h.DB.GetProjectDockerThresholds(r.Context(), projectID)
	if err != nil {
		h.Log.Error("get docker settings", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	if thresholds == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"thresholds": nil})
		return
	}

	var parsed interface{}
	if err := json.Unmarshal(thresholds, &parsed); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"thresholds": nil})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"thresholds": parsed})
}

func (h *Handler) updateDockerSettings(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var body struct {
		Thresholds json.RawMessage `json:"thresholds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.DB.UpdateProjectDockerThresholds(r.Context(), projectID, body.Thresholds); err != nil {
		h.Log.Error("update docker settings", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"thresholds": json.RawMessage(body.Thresholds)})
}

func (h *Handler) listAllContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := h.DB.ListAllContainers(r.Context())
	if err != nil {
		h.Log.Error("list all containers", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]containerResponse, len(containers))
	for i := range containers {
		resp[i] = toContainerResponse(&containers[i])
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"containers": resp})
}
