package reaper

import (
	"testing"
)

func TestValidateDBName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"hq", false},
		{"beads", false},
		{"gt", false},
		{"test_db_123", false},
		{"", true},
		{"drop table", true},
		{"db;--", true},
		{"db`name", true},
		{"../etc/passwd", true},
	}
	for _, tt := range tests {
		err := ValidateDBName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDBName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestDefaultDatabases(t *testing.T) {
	if len(DefaultDatabases) == 0 {
		t.Error("DefaultDatabases should not be empty")
	}
	for _, db := range DefaultDatabases {
		if err := ValidateDBName(db); err != nil {
			t.Errorf("DefaultDatabases contains invalid name %q: %v", db, err)
		}
	}
}

func TestFormatJSON(t *testing.T) {
	result := FormatJSON(map[string]int{"count": 42})
	if result == "" {
		t.Error("FormatJSON should not return empty string")
	}
	if result[0] != '{' {
		t.Errorf("FormatJSON should return JSON object, got %q", result[:10])
	}
}

func TestParentJoinClause(t *testing.T) {
	join := parentJoinClause("testdb")
	if join == "" {
		t.Error("parentJoinClause should not return empty string")
	}
	if !contains(join, "LEFT JOIN") {
		t.Error("parentJoinClause should use LEFT JOIN")
	}
	if !contains(join, "wisp_dependencies") {
		t.Error("parentJoinClause should reference wisp_dependencies")
	}
	if !contains(join, "parent-child") {
		t.Error("parentJoinClause should filter on parent-child type")
	}
}

func TestParentWhereClause(t *testing.T) {
	// Should contain all three eligibility branches: orphan, closed parent, dangling ref.
	if !contains(parentWhereClause, "wd.issue_id IS NULL") {
		t.Error("parentWhereClause should check for orphans (no parent dep)")
	}
	if !contains(parentWhereClause, "parent.status = 'closed'") {
		t.Error("parentWhereClause should check parent status is closed")
	}
	if !contains(parentWhereClause, "parent.id IS NULL") {
		t.Error("parentWhereClause should handle dangling parent refs")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
