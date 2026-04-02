package dashboard

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

// requireDashRole checks the authenticated user's role (from X-Account-Role
// header set by requireAuth) against the given minimum role. On failure it
// renders a 403 flash and redirects to the dashboard index.
func requireDashRole(minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountRole := r.Header.Get("X-Account-Role")
		if !hasMinRole(accountRole, minRole) {
			http.Error(w, "Forbidden: requires "+minRole+" role or higher", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
