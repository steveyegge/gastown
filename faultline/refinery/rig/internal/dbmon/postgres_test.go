package dbmon

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// mockPGDB implements pgConnector for testing.
type mockPGDB struct {
	pingErr      error
	rowResults   map[string]*mockRowScanner // keyed by query prefix
	rowsResults  map[string]*mockPGRows
	rowsErrors   map[string]error
	rowResultSeq []*mockRowScanner // consumed in order for QueryRowContext
	rowIdx       int
}

func newMockPGDB() *mockPGDB {
	return &mockPGDB{
		rowResults:  make(map[string]*mockRowScanner),
		rowsResults: make(map[string]*mockPGRows),
		rowsErrors:  make(map[string]error),
	}
}

func (m *mockPGDB) PingContext(_ context.Context) error {
	return m.pingErr
}

func (m *mockPGDB) QueryRowContext(_ context.Context, _ string, _ ...any) rowScanner {
	if m.rowIdx < len(m.rowResultSeq) {
		r := m.rowResultSeq[m.rowIdx]
		m.rowIdx++
		return r
	}
	return &mockRowScanner{err: errors.New("no mock row configured")}
}

func (m *mockPGDB) QueryContext(_ context.Context, _ string, _ ...any) (pgRows, error) {
	// Return errors or empty rows by default.
	return &mockPGRows{}, nil
}

func (m *mockPGDB) Close() error { return nil }

// mockRowScanner implements rowScanner for testing.
type mockRowScanner struct {
	vals []any
	err  error
}

func (r *mockRowScanner) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i, d := range dest {
		if i >= len(r.vals) {
			break
		}
		switch v := d.(type) {
		case *int:
			switch src := r.vals[i].(type) {
			case int:
				*v = src
			case int64:
				*v = int(src)
			}
		case *int64:
			switch src := r.vals[i].(type) {
			case int64:
				*v = src
			case int:
				*v = int64(src)
			}
		case *float64:
			*v = r.vals[i].(float64)
		case *string:
			*v = r.vals[i].(string)
		case *sql.NullString:
			if r.vals[i] == nil {
				v.Valid = false
			} else {
				v.String = r.vals[i].(string)
				v.Valid = true
			}
		}
	}
	return nil
}

// mockPGRows implements pgRows for testing.
type mockPGRows struct {
	data [][]any
	idx  int
}

func (r *mockPGRows) Next() bool {
	return r.idx < len(r.data)
}

func (r *mockPGRows) Scan(dest ...any) error {
	if r.idx >= len(r.data) {
		return errors.New("no more rows")
	}
	row := r.data[r.idx]
	r.idx++
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch v := d.(type) {
		case *int:
			switch src := row[i].(type) {
			case int:
				*v = src
			case int64:
				*v = int(src)
			}
		case *int64:
			switch src := row[i].(type) {
			case int64:
				*v = src
			case int:
				*v = int64(src)
			}
		case *float64:
			*v = row[i].(float64)
		case *string:
			*v = row[i].(string)
		case *sql.NullString:
			if row[i] == nil {
				v.Valid = false
			} else {
				v.String = row[i].(string)
				v.Valid = true
			}
		}
	}
	return nil
}

func (r *mockPGRows) Close() error { return nil }

// queryDispatchDB routes QueryRowContext and QueryContext based on call order,
// allowing fine-grained control of each query's response.
type queryDispatchDB struct {
	pingErr      error
	rowSeq       []*mockRowScanner
	rowIdx       int
	rowsSeq      []rowsResponse
	rowsIdx      int
}

type rowsResponse struct {
	rows pgRows
	err  error
}

func (d *queryDispatchDB) PingContext(_ context.Context) error { return d.pingErr }

func (d *queryDispatchDB) QueryRowContext(_ context.Context, _ string, _ ...any) rowScanner {
	if d.rowIdx < len(d.rowSeq) {
		r := d.rowSeq[d.rowIdx]
		d.rowIdx++
		return r
	}
	return &mockRowScanner{err: errors.New("unexpected QueryRowContext call")}
}

func (d *queryDispatchDB) QueryContext(_ context.Context, _ string, _ ...any) (pgRows, error) {
	if d.rowsIdx < len(d.rowsSeq) {
		r := d.rowsSeq[d.rowsIdx]
		d.rowsIdx++
		return r.rows, r.err
	}
	return &mockPGRows{}, nil
}

func (d *queryDispatchDB) Close() error { return nil }

// --- Tests ---

func TestParsePGThresholds_Defaults(t *testing.T) {
	th := parsePGThresholds(sql.NullString{})
	if th.ConnectionUsagePct != defaultConnectionUsagePct {
		t.Errorf("expected default connection usage %v, got %v", defaultConnectionUsagePct, th.ConnectionUsagePct)
	}
	if th.LongQuerySec != defaultLongQuerySec {
		t.Errorf("expected default long query %v, got %v", defaultLongQuerySec, th.LongQuerySec)
	}
	if th.ReplicationLagSec != defaultReplicationLagSec {
		t.Errorf("expected default replication lag %v, got %v", defaultReplicationLagSec, th.ReplicationLagSec)
	}
	if th.DeadTupleRatioPct != defaultDeadTupleRatioPct {
		t.Errorf("expected default dead tuple ratio %v, got %v", defaultDeadTupleRatioPct, th.DeadTupleRatioPct)
	}
}

func TestParsePGThresholds_CustomValues(t *testing.T) {
	raw := sql.NullString{
		String: `{"connection_usage_pct": 90, "long_query_sec": 60, "replication_lag_sec": 5}`,
		Valid:  true,
	}
	th := parsePGThresholds(raw)
	if th.ConnectionUsagePct != 90 {
		t.Errorf("expected connection usage 90, got %v", th.ConnectionUsagePct)
	}
	if th.LongQuerySec != 60 {
		t.Errorf("expected long query 60, got %v", th.LongQuerySec)
	}
	if th.ReplicationLagSec != 5 {
		t.Errorf("expected replication lag 5, got %v", th.ReplicationLagSec)
	}
	if th.DeadTupleRatioPct != defaultDeadTupleRatioPct {
		t.Errorf("expected default dead tuple ratio, got %v", th.DeadTupleRatioPct)
	}
}

func TestParsePGThresholds_InvalidJSON(t *testing.T) {
	raw := sql.NullString{String: "not json", Valid: true}
	th := parsePGThresholds(raw)
	if th.ConnectionUsagePct != defaultConnectionUsagePct {
		t.Errorf("expected default on invalid JSON, got %v", th.ConnectionUsagePct)
	}
}

func TestCheckPostgres_ConnectionFailure(t *testing.T) {
	checker := newPostgresCheckerWith(func(_ string) (pgConnector, error) {
		return nil, errors.New("connection refused")
	})

	target := DatabaseTarget{
		ID:               "pg-1",
		Name:             "test-pg",
		DBType:           "postgres",
		ConnectionString: "postgres://localhost/test",
	}

	results := checker(context.Background(), target)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckCritical {
		t.Errorf("expected critical, got %v", results[0].Status)
	}
	if results[0].CheckType != "connection" {
		t.Errorf("expected 'connection', got %s", results[0].CheckType)
	}
}

func TestCheckPGConnection_Success(t *testing.T) {
	db := &queryDispatchDB{}
	now := time.Now().UTC()

	result := checkPGConnection(context.Background(), db, "pg-1", now)
	if result.Status != CheckOK {
		t.Errorf("expected ok, got %v", result.Status)
	}
	if result.Value == nil {
		t.Error("expected latency value")
	}
}

func TestCheckPGConnection_Failure(t *testing.T) {
	db := &queryDispatchDB{pingErr: errors.New("timeout")}
	now := time.Now().UTC()

	result := checkPGConnection(context.Background(), db, "pg-1", now)
	if result.Status != CheckCritical {
		t.Errorf("expected critical, got %v", result.Status)
	}
}

func TestCheckPGConnectionUsage_OK(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{vals: []any{100}},  // max_connections = 100
			{vals: []any{20}},   // active = 20 (20%)
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGConnectionUsage(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckOK {
		t.Errorf("expected ok, got %v", results[0].Status)
	}
	if results[0].Value == nil || *results[0].Value != 20.0 {
		t.Errorf("expected 20.0%%, got %v", results[0].Value)
	}
}

func TestCheckPGConnectionUsage_Warning(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{vals: []any{100}}, // max_connections = 100
			{vals: []any{85}},  // active = 85 (85% > 80% threshold)
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGConnectionUsage(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckWarning {
		t.Errorf("expected warning, got %v", results[0].Status)
	}
}

func TestCheckPGConnectionUsage_QueryError(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{err: errors.New("permission denied")},
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGConnectionUsage(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckWarning {
		t.Errorf("expected warning on query error, got %v", results[0].Status)
	}
}

func TestCheckPGDeadlocks_OK(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{vals: []any{int64(5)}}, // 5 total deadlocks
		},
	}
	now := time.Now().UTC()

	result := checkPGDeadlocks(context.Background(), db, "pg-1", now)
	if result.Status != CheckOK {
		t.Errorf("expected ok, got %v", result.Status)
	}
	if result.Value == nil || *result.Value != 5.0 {
		t.Errorf("expected value 5, got %v", result.Value)
	}
}

func TestCheckPGDeadTuples_OK(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{vals: []any{int64(10000), int64(100)}}, // 100 dead / 10100 total = ~1%
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	result := checkPGDeadTuples(context.Background(), db, "pg-1", now, th)
	if result.Status != CheckOK {
		t.Errorf("expected ok, got %v", result.Status)
	}
}

func TestCheckPGDeadTuples_Warning(t *testing.T) {
	db := &queryDispatchDB{
		rowSeq: []*mockRowScanner{
			{vals: []any{int64(1000), int64(500)}}, // 500 dead / 1500 total = 33%
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	result := checkPGDeadTuples(context.Background(), db, "pg-1", now, th)
	if result.Status != CheckWarning {
		t.Errorf("expected warning for high dead tuple ratio, got %v", result.Status)
	}
}

func TestCheckPGReplicationLag_NoReplicas(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{}}, // empty result set
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGReplicationLag(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckOK {
		t.Errorf("expected ok for no replicas, got %v", results[0].Status)
	}
}

func TestCheckPGReplicationLag_OK(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{data: [][]any{
				{"10.0.0.2", 2.5}, // lag = 2.5s, below 10s threshold
			}}},
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGReplicationLag(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckOK {
		t.Errorf("expected ok, got %v", results[0].Status)
	}
}

func TestCheckPGReplicationLag_Warning(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{data: [][]any{
				{"10.0.0.2", 15.0}, // lag = 15s, above 10s threshold
			}}},
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGReplicationLag(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckWarning {
		t.Errorf("expected warning for high lag, got %v", results[0].Status)
	}
}

func TestCheckPGReplicationLag_QueryError(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{err: errors.New("permission denied")},
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGReplicationLag(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckOK {
		t.Errorf("expected ok (graceful skip) on query error, got %v", results[0].Status)
	}
}

func TestCheckPGLongQueries_None(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{}}, // no long queries
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGLongQueries(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckOK {
		t.Errorf("expected ok, got %v", results[0].Status)
	}
}

func TestCheckPGLongQueries_Found(t *testing.T) {
	db := &queryDispatchDB{
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{data: [][]any{
				{123, 45.0, "SELECT * FROM big_table"},
				{456, 60.0, "UPDATE slow_table SET ..."},
			}}},
		},
	}
	now := time.Now().UTC()
	th := parsePGThresholds(sql.NullString{})

	results := checkPGLongQueries(context.Background(), db, "pg-1", now, th)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != CheckWarning {
		t.Errorf("expected warning, got %v", results[0].Status)
	}
	if results[0].Value == nil || *results[0].Value != 60.0 {
		t.Errorf("expected max duration 60.0, got %v", results[0].Value)
	}
}

func TestCheckPostgres_AllChecksHealthy(t *testing.T) {
	// Full happy-path: all checks return ok.
	db := &queryDispatchDB{
		// QueryRowContext calls (in order):
		// 1. SHOW max_connections → 100
		// 2. SELECT count(*) FROM pg_stat_activity → 10
		// 3. SELECT COALESCE(SUM(deadlocks)...) → 0
		// 4. SELECT COALESCE(SUM(n_live_tup)...) → 10000, 50
		rowSeq: []*mockRowScanner{
			{vals: []any{100}},
			{vals: []any{10}},
			{vals: []any{int64(0)}},
			{vals: []any{int64(10000), int64(50)}},
		},
		// QueryContext calls (in order):
		// 1. pg_stat_replication → no replicas
		// 2. pg_stat_activity long queries → none
		rowsSeq: []rowsResponse{
			{rows: &mockPGRows{}},
			{rows: &mockPGRows{}},
		},
	}

	checker := newPostgresCheckerWith(func(_ string) (pgConnector, error) {
		return db, nil
	})

	target := DatabaseTarget{
		ID:               "pg-1",
		Name:             "test-pg",
		DBType:           "postgres",
		ConnectionString: "postgres://localhost/test",
	}

	results := checker(context.Background(), target)

	// Collect check types.
	checkTypes := make(map[string]CheckStatus)
	for _, r := range results {
		checkTypes[r.CheckType] = r.Status
	}

	expected := map[string]CheckStatus{
		"connection":       CheckOK,
		"connection_usage": CheckOK,
		"deadlocks":        CheckOK,
		"dead_tuples":      CheckOK,
		"replication_lag":  CheckOK,
		"long_queries":     CheckOK,
	}

	for ct, wantStatus := range expected {
		got, ok := checkTypes[ct]
		if !ok {
			t.Errorf("missing check type: %s", ct)
			continue
		}
		if got != wantStatus {
			t.Errorf("check %s: expected %v, got %v", ct, wantStatus, got)
		}
	}
}

func TestCheckPostgres_IntegrationWithMonitor(t *testing.T) {
	provider := newMockProvider([]DatabaseTarget{
		{ID: "pg-1", Name: "test-pg", DBType: "postgres", Enabled: true, CheckIntervalS: 1},
	})

	m := New(provider, testLogger(), 5*time.Second)

	m.RegisterChecker("postgres", newPostgresCheckerWith(func(_ string) (pgConnector, error) {
		return nil, errors.New("connection refused")
	}))

	m.OnStateChange = func(_ DatabaseTarget, _, _ Status, _ []CheckResult) {}

	m.tick(context.Background())

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("expected check results to be persisted")
	}
	if checks[0].Status != CheckCritical {
		t.Errorf("expected critical status, got %v", checks[0].Status)
	}

	state := provider.getState("pg-1")
	if state == nil {
		t.Fatal("expected persisted state")
	}
	if state.Status != StatusDown {
		t.Errorf("expected down status, got %v", state.Status)
	}
}

func TestNewPostgresChecker_ReturnsCheckFunc(t *testing.T) {
	checker := NewPostgresChecker()
	if checker == nil {
		t.Fatal("expected non-nil checker")
	}
}
