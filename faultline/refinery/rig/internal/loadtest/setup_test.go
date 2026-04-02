package loadtest

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestSetupDatabase(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	db, err := sql.Open("mysql", "root@tcp(127.0.0.1:3307)/")
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS faultline_loadtest")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	t.Log("faultline_loadtest database ready")
}
