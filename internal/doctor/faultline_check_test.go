package doctor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFaultlineDSNCheck_NotSet(t *testing.T) {
	t.Setenv("FAULTLINE_DSN", "")
	check := NewFaultlineDSNCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusWarning {
		t.Errorf("expected warning when DSN not set, got %v", result.Status)
	}
}

func TestFaultlineDSNCheck_Set(t *testing.T) {
	t.Setenv("FAULTLINE_DSN", "http://key@localhost:8080/2")
	check := NewFaultlineDSNCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusOK {
		t.Errorf("expected OK when DSN is set, got %v", result.Status)
	}
}

func TestFaultlineReachableCheck_NoDSN(t *testing.T) {
	t.Setenv("FAULTLINE_DSN", "")
	check := NewFaultlineReachableCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusWarning {
		t.Errorf("expected warning when DSN not set, got %v", result.Status)
	}
}

func TestFaultlineReachableCheck_ServerUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("FAULTLINE_DSN", "http://key@localhost:8080/2")
	t.Setenv("FAULTLINE_API_URL", srv.URL)

	check := NewFaultlineReachableCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusOK {
		t.Errorf("expected OK when server is up, got %v: %s", result.Status, result.Message)
	}
}

func TestFaultlineReachableCheck_ServerDown(t *testing.T) {
	t.Setenv("FAULTLINE_DSN", "http://key@localhost:8080/2")
	t.Setenv("FAULTLINE_API_URL", "http://localhost:1") // Nothing listening

	check := NewFaultlineReachableCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusWarning {
		t.Errorf("expected warning when server is down, got %v", result.Status)
	}
}

func TestFaultlineReachableCheck_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("FAULTLINE_DSN", "http://key@localhost:8080/2")
	t.Setenv("FAULTLINE_API_URL", srv.URL)

	check := NewFaultlineReachableCheck()
	result := check.Run(&CheckContext{})
	if result.Status != StatusWarning {
		t.Errorf("expected warning when server returns 500, got %v", result.Status)
	}
}

func TestFaultlineDSNCheck_CanFix(t *testing.T) {
	check := NewFaultlineDSNCheck()
	if check.CanFix() {
		t.Error("DSN check should not be auto-fixable")
	}
}

func TestFaultlineReachableCheck_CanFix(t *testing.T) {
	check := NewFaultlineReachableCheck()
	if check.CanFix() {
		t.Error("reachable check should not be auto-fixable")
	}
}

// Ensure unused import doesn't cause issues.
var _ = os.Getenv
