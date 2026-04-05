package clouddb

import (
	"testing"
)

func TestParseMigrationName(t *testing.T) {
	tests := []struct {
		input   string
		wantVer int
		wantN   string
		wantOK  bool
	}{
		{"001_projects.sql", 1, "projects", true},
		{"010_big_one.sql", 10, "big_one", true},
		{"999_last.sql", 999, "last", true},
		{"bad.sql", 0, "", false},
		{"_nonum.sql", 0, "", false},
		{"notasql.txt", 0, "", false},
	}
	for _, tt := range tests {
		ver, name, ok := parseMigrationName(tt.input)
		if ok != tt.wantOK || ver != tt.wantVer || name != tt.wantN {
			t.Errorf("parseMigrationName(%q) = (%d, %q, %v); want (%d, %q, %v)",
				tt.input, ver, name, ok, tt.wantVer, tt.wantN, tt.wantOK)
		}
	}
}

func TestSplitStatements(t *testing.T) {
	sql := `CREATE TABLE foo (id INT);
-- comment
CREATE TABLE bar (id INT);
`
	got := splitStatements(sql)
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(got), got)
	}
	if got[0] != "CREATE TABLE foo (id INT)" {
		t.Errorf("stmt[0] = %q", got[0])
	}
	if got[1] != "-- comment\nCREATE TABLE bar (id INT)" {
		t.Errorf("stmt[1] = %q", got[1])
	}
}

func TestListMigrations(t *testing.T) {
	migs, err := listMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) < 5 {
		t.Fatalf("expected at least 5 embedded migrations, got %d", len(migs))
	}
	// Verify sorted order.
	for i := 1; i < len(migs); i++ {
		if migs[i].version <= migs[i-1].version {
			t.Errorf("migrations not sorted: %d after %d", migs[i].version, migs[i-1].version)
		}
	}
	// Verify first migration.
	if migs[0].version != 1 || migs[0].name != "projects" {
		t.Errorf("first migration = %+v; want version=1, name=projects", migs[0])
	}
}

func TestExtractDBName(t *testing.T) {
	tests := []struct {
		dsn  string
		want string
	}{
		{"root@tcp(127.0.0.1:3307)/faultline_cloud", "faultline_cloud"},
		{"root@tcp(127.0.0.1:3307)/faultline_cloud?parseTime=true", "faultline_cloud"},
		{"root@tcp(host)/", ""},
		{"nodatabase", ""},
	}
	for _, tt := range tests {
		got := extractDBName(tt.dsn)
		if got != tt.want {
			t.Errorf("extractDBName(%q) = %q; want %q", tt.dsn, got, tt.want)
		}
	}
}
