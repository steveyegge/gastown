package api

import (
	"io"
	"net/http"

	"github.com/outdoorsea/faultline/internal/sourcemap"
)

// SourceMapStore is the source map store used by the API handler.
// Set this before calling RegisterRoutes if source map support is desired.
var sourceMapStore *sourcemap.Store

// SetSourceMapStore sets the global source map store for the API.
func SetSourceMapStore(s *sourcemap.Store) {
	sourceMapStore = s
}

// registerSourceMapRoutes registers source map upload/list/delete routes.
func (h *Handler) registerSourceMapRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	mux.HandleFunc("POST /api/v1/projects/{project_id}/releases/{release}/files/", auth(projAccess(admin(h.uploadSourceMap))))
	mux.HandleFunc("POST /api/v1/projects/{project_id}/releases/{release}/files", auth(projAccess(admin(h.uploadSourceMap))))
	mux.HandleFunc("GET /api/v1/projects/{project_id}/releases/{release}/files/", auth(projAccess(h.listSourceMaps)))
	mux.HandleFunc("GET /api/v1/projects/{project_id}/releases/{release}/files", auth(projAccess(h.listSourceMaps)))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/releases/{release}/files/", auth(projAccess(admin(h.deleteSourceMaps))))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/releases/{release}/files", auth(projAccess(admin(h.deleteSourceMaps))))
}

// uploadSourceMap handles POST /api/projects/{project_id}/releases/{release}/files/
// Accepts multipart/form-data with a "file" field and "name" query param.
func (h *Handler) uploadSourceMap(w http.ResponseWriter, r *http.Request) {
	if sourceMapStore == nil {
		writeErr(w, http.StatusServiceUnavailable, "source map storage not configured")
		return
	}

	projectID := pathInt64(r, "project_id")
	release := r.PathValue("release")
	if projectID <= 0 || release == "" {
		writeErr(w, http.StatusBadRequest, "invalid project_id or release")
		return
	}

	name := r.URL.Query().Get("name")

	// Parse multipart form (max 32 MB).
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'file' field in multipart form")
		return
	}
	defer func() { _ = file.Close() }()

	if name == "" {
		name = header.Filename
	}
	if name == "" {
		writeErr(w, http.StatusBadRequest, "filename required via 'name' query param or multipart filename")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		h.Log.Error("read source map upload", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to read file")
		return
	}

	if err := sourceMapStore.Save(projectID, release, name, data); err != nil {
		h.Log.Error("save source map", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to save source map")
		return
	}

	h.Log.Info("source map uploaded",
		"project_id", projectID,
		"release", release,
		"name", name,
		"size", len(data),
	)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"name": name,
		"size": len(data),
	})
}

// listSourceMaps handles GET /api/projects/{project_id}/releases/{release}/files/
func (h *Handler) listSourceMaps(w http.ResponseWriter, r *http.Request) {
	if sourceMapStore == nil {
		writeErr(w, http.StatusServiceUnavailable, "source map storage not configured")
		return
	}

	projectID := pathInt64(r, "project_id")
	release := r.PathValue("release")
	if projectID <= 0 || release == "" {
		writeErr(w, http.StatusBadRequest, "invalid project_id or release")
		return
	}

	files, err := sourceMapStore.List(projectID, release)
	if err != nil {
		h.Log.Error("list source maps", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to list source maps")
		return
	}
	if files == nil {
		files = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"files": files,
	})
}

// deleteSourceMaps handles DELETE /api/projects/{project_id}/releases/{release}/files/
func (h *Handler) deleteSourceMaps(w http.ResponseWriter, r *http.Request) {
	if sourceMapStore == nil {
		writeErr(w, http.StatusServiceUnavailable, "source map storage not configured")
		return
	}

	projectID := pathInt64(r, "project_id")
	release := r.PathValue("release")
	if projectID <= 0 || release == "" {
		writeErr(w, http.StatusBadRequest, "invalid project_id or release")
		return
	}

	if err := sourceMapStore.Delete(projectID, release); err != nil {
		h.Log.Error("delete source maps", "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to delete source maps")
		return
	}

	h.Log.Info("source maps deleted", "project_id", projectID, "release", release)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
