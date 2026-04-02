package api

import "net/http"

// roleLevel maps role strings to numeric levels for comparison.
// Higher level = more permissions.
var roleLevel = map[string]int{
	"viewer": 1,
	"member": 2,
	"admin":  3,
	"owner":  4,
}

// hasMinRole reports whether the account role meets or exceeds the minimum.
func hasMinRole(accountRole, minRole string) bool {
	return roleLevel[accountRole] >= roleLevel[minRole]
}

// requireRole returns a middleware that checks the authenticated user's role
// (from X-Account-Role header, set by requireBearer/requireAuth) against the
// given minimum role in the hierarchy: owner > admin > member > viewer.
func requireRole(minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountRole := r.Header.Get("X-Account-Role")
		if !hasMinRole(accountRole, minRole) {
			writeErr(w, http.StatusForbidden, "insufficient permissions: requires "+minRole+" role or higher")
			return
		}
		next(w, r)
	}
}

// headerAccountRole reads the account role from the request header.
func headerAccountRole(r *http.Request) string {
	return r.Header.Get("X-Account-Role")
}
