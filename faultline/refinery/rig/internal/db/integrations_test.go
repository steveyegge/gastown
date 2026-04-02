package db

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func openIntegrationsTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_integ_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM integrations_config")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM projects")
		_ = d.Close()
	})
	return d
}

func seedProject(t *testing.T, d *DB) int64 {
	t.Helper()
	ctx := context.Background()
	err := d.EnsureProject(ctx, 1, "test-project", "test-project", "testkey123")
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return 1
}

func TestInsertAndGetIntegration(t *testing.T) {
	d := openIntegrationsTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, d)

	cfg := &IntegrationConfig{
		ProjectID:       projectID,
		IntegrationType: "github_issues",
		Name:            "GitHub Issues",
		Enabled:         true,
		Config:          json.RawMessage(`{"owner":"org","repo":"faultline","labels":["bug"]}`),
	}

	if err := d.InsertIntegration(ctx, cfg); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected ID to be set")
	}

	got, err := d.GetIntegration(ctx, projectID, cfg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "GitHub Issues" {
		t.Errorf("expected name 'GitHub Issues', got %q", got.Name)
	}
	if got.IntegrationType != "github_issues" {
		t.Errorf("expected type 'github_issues', got %q", got.IntegrationType)
	}
	if !got.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestListIntegrations(t *testing.T) {
	d := openIntegrationsTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, d)

	_ = d.InsertIntegration(ctx, &IntegrationConfig{
		ProjectID: projectID, IntegrationType: "github_issues", Name: "GH", Enabled: true,
		Config: json.RawMessage(`{}`),
	})
	_ = d.InsertIntegration(ctx, &IntegrationConfig{
		ProjectID: projectID, IntegrationType: "pagerduty", Name: "PD", Enabled: false,
		Config: json.RawMessage(`{}`),
	})

	all, err := d.ListIntegrations(ctx, projectID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 integrations, got %d", len(all))
	}

	enabled, err := d.ListEnabledIntegrations(ctx, projectID)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled, got %d", len(enabled))
	}
	if enabled[0].Name != "GH" {
		t.Errorf("expected 'GH', got %q", enabled[0].Name)
	}
}

func TestUpdateIntegration(t *testing.T) {
	d := openIntegrationsTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, d)

	cfg := &IntegrationConfig{
		ProjectID: projectID, IntegrationType: "jira", Name: "Jira Cloud", Enabled: true,
		Config: json.RawMessage(`{"project":"FL"}`),
	}
	_ = d.InsertIntegration(ctx, cfg)

	cfg.Name = "Jira Cloud Updated"
	cfg.Enabled = false
	if err := d.UpdateIntegration(ctx, cfg); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := d.GetIntegration(ctx, projectID, cfg.ID)
	if got.Name != "Jira Cloud Updated" {
		t.Errorf("expected updated name, got %q", got.Name)
	}
	if got.Enabled {
		t.Error("expected disabled after update")
	}
}

func TestDeleteIntegration(t *testing.T) {
	d := openIntegrationsTestDB(t)
	ctx := context.Background()
	projectID := seedProject(t, d)

	cfg := &IntegrationConfig{
		ProjectID: projectID, IntegrationType: "linear", Name: "Linear", Enabled: true,
		Config: json.RawMessage(`{}`),
	}
	_ = d.InsertIntegration(ctx, cfg)

	if err := d.DeleteIntegration(ctx, projectID, cfg.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := d.GetIntegration(ctx, projectID, cfg.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}
