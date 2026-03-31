package ingest

import (
	"fmt"
	"net/http"
	"strings"
)

// ProjectAuth maps public keys to project IDs and optional rig names.
type ProjectAuth struct {
	// keyToProject maps sentry_key (public DSN key) -> project_id.
	keyToProject map[string]int64
	// projectToRig maps project_id -> target rig name for Gas Town integration.
	projectToRig map[int64]string
}

// NewProjectAuth creates an authenticator from "project_id:public_key[:rig]" pairs.
// The rig component is optional — projects without a rig won't trigger bead creation.
func NewProjectAuth(pairs []string) (*ProjectAuth, error) {
	keys := make(map[string]int64, len(pairs))
	rigs := make(map[int64]string, len(pairs))
	for _, p := range pairs {
		parts := strings.SplitN(p, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid project auth pair: %q (want project_id:public_key[:rig])", p)
		}
		var id int64
		if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid project id in %q: %w", p, err)
		}
		keys[parts[1]] = id
		if len(parts) == 3 && parts[2] != "" {
			rigs[id] = parts[2]
		}
	}
	return &ProjectAuth{keyToProject: keys, projectToRig: rigs}, nil
}

// RigForProject returns the target Gas Town rig for a project, or "" if none configured.
func (a *ProjectAuth) RigForProject(projectID int64) string {
	return a.projectToRig[projectID]
}

// Authenticate extracts the sentry_key from the request and returns the project ID.
// It checks X-Sentry-Auth header, Authorization header, and sentry_key query param.
func (a *ProjectAuth) Authenticate(r *http.Request) (int64, error) {
	key := extractKey(r)
	if key == "" {
		return 0, fmt.Errorf("missing sentry authentication")
	}
	pid, ok := a.keyToProject[key]
	if !ok {
		return 0, fmt.Errorf("invalid sentry key")
	}
	return pid, nil
}

// extractKey pulls the sentry_key from the request in priority order:
// 1. X-Sentry-Auth header
// 2. Authorization header
// 3. sentry_key query parameter
func extractKey(r *http.Request) string {
	for _, hdr := range []string{"X-Sentry-Auth", "Authorization"} {
		if v := r.Header.Get(hdr); v != "" {
			if key := parseAuthHeader(v); key != "" {
				return key
			}
		}
	}
	return r.URL.Query().Get("sentry_key")
}

// parseAuthHeader parses "Sentry sentry_key=VALUE, ..." format.
func parseAuthHeader(header string) string {
	header = strings.TrimPrefix(header, "Sentry ")
	header = strings.TrimPrefix(header, "sentry ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "sentry_key=") {
			return strings.TrimPrefix(part, "sentry_key=")
		}
	}
	return ""
}
