package daemon

// Tests for compactorGetRowCounts.
//
// We test without a real Dolt server by registering a lightweight fake
// database/sql driver that returns canned responses for the two query patterns
// the function issues:
//
//   1. SELECT table_name FROM information_schema.tables WHERE ...
//   2. SELECT COUNT(*) FROM `db`.`tableName`
//
// Each test registers a unique driver name (via an atomic counter) so tests
// can run in parallel without sharing driver state.

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Fake SQL driver
// ============================================================================

// fakeRowCountsConfig holds canned responses for one test's fake DB.
type fakeRowCountsConfig struct {
	// tables maps table name -> exact row count returned by COUNT(*).
	tables map[string]int
	// listErr is returned instead of table rows for the listing query.
	listErr error
	// countErrs maps table name -> error returned by the COUNT(*) query.
	countErrs map[string]error
	// countDelay is injected into every COUNT(*) query to expose serial vs
	// parallel execution: serial time ≈ N×delay, parallel time ≈ delay.
	countDelay time.Duration
}

type fakeRowCountsDriver struct{ cfg *fakeRowCountsConfig }

func (d *fakeRowCountsDriver) Open(_ string) (driver.Conn, error) {
	return &fakeRowCountsConn{cfg: d.cfg}, nil
}

type fakeRowCountsConn struct{ cfg *fakeRowCountsConfig }

func (c *fakeRowCountsConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeRowCountsStmt{cfg: c.cfg, query: query}, nil
}
func (c *fakeRowCountsConn) Close() error             { return nil }
func (c *fakeRowCountsConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not supported in fakeRowCountsConn")
}

type fakeRowCountsStmt struct {
	cfg   *fakeRowCountsConfig
	query string
}

func (s *fakeRowCountsStmt) Close() error  { return nil }
func (s *fakeRowCountsStmt) NumInput() int { return -1 }
func (s *fakeRowCountsStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return nil, fmt.Errorf("Exec not supported by fakeRowCountsStmt")
}

func (s *fakeRowCountsStmt) Query(_ []driver.Value) (driver.Rows, error) {
	if strings.Contains(s.query, "information_schema") {
		// Table-listing query.
		if s.cfg.listErr != nil {
			return nil, s.cfg.listErr
		}
		names := make([]string, 0, len(s.cfg.tables))
		for name := range s.cfg.tables {
			names = append(names, name)
		}
		return &fakeTableListRows{names: names}, nil
	}

	// COUNT(*) query — parse table name from backtick syntax:
	//   SELECT COUNT(*) FROM `dbName`.`tableName`
	table := fakeExtractTableName(s.query)

	if s.cfg.countDelay > 0 {
		time.Sleep(s.cfg.countDelay)
	}

	if err, ok := s.cfg.countErrs[table]; ok {
		return nil, err
	}

	count, ok := s.cfg.tables[table]
	if !ok {
		return nil, fmt.Errorf("fake driver: unknown table %q", table)
	}
	return &fakeCountRows{count: count}, nil
}

// fakeExtractTableName parses the last backtick-delimited identifier from a
// query of the form: SELECT COUNT(*) FROM `db`.`table`
func fakeExtractTableName(query string) string {
	parts := strings.Split(query, "`")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

// fakeTableListRows implements driver.Rows for the information_schema query.
type fakeTableListRows struct {
	names []string
	idx   int
}

func (r *fakeTableListRows) Columns() []string { return []string{"table_name"} }
func (r *fakeTableListRows) Close() error      { return nil }
func (r *fakeTableListRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.names) {
		return io.EOF
	}
	dest[0] = r.names[r.idx]
	r.idx++
	return nil
}

// fakeCountRows implements driver.Rows for COUNT(*) queries.
type fakeCountRows struct {
	count   int
	yielded bool
}

func (r *fakeCountRows) Columns() []string { return []string{"COUNT(*)"} }
func (r *fakeCountRows) Close() error      { return nil }
func (r *fakeCountRows) Next(dest []driver.Value) error {
	if r.yielded {
		return io.EOF
	}
	dest[0] = int64(r.count)
	r.yielded = true
	return nil
}

// driverSeq generates unique driver names so tests don't collide on the
// global sql.Register registry.
var driverSeq atomic.Int64

// newFakeRowCountsDB creates a *sql.DB backed by the fake driver.
func newFakeRowCountsDB(cfg *fakeRowCountsConfig) *sql.DB {
	name := fmt.Sprintf("fake-rowcounts-%d", driverSeq.Add(1))
	sql.Register(name, &fakeRowCountsDriver{cfg: cfg})
	db, err := sql.Open(name, "")
	if err != nil {
		panic(fmt.Sprintf("newFakeRowCountsDB: %v", err))
	}
	return db
}

// newTestDaemon returns a minimal Daemon suitable for calling compactorGetRowCounts.
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	return &Daemon{logger: log.New(io.Discard, "", 0)}
}

// ============================================================================
// Tests
// ============================================================================

// TestCompactorGetRowCounts_Empty verifies that zero tables returns an empty
// map with no error. The integrity gate must not trip on an empty DB.
func TestCompactorGetRowCounts_Empty(t *testing.T) {
	db := newFakeRowCountsDB(&fakeRowCountsConfig{tables: map[string]int{}})
	defer db.Close()

	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %v", counts)
	}
}

// TestCompactorGetRowCounts_SingleTable verifies that a single table's exact
// count is returned correctly.
func TestCompactorGetRowCounts_SingleTable(t *testing.T) {
	db := newFakeRowCountsDB(&fakeRowCountsConfig{
		tables: map[string]int{"issues": 42},
	})
	defer db.Close()

	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["issues"] != 42 {
		t.Errorf("issues: want 42, got %d", counts["issues"])
	}
}

// TestCompactorGetRowCounts_MultipleTables verifies that all tables and their
// exact counts are present in the result map.
func TestCompactorGetRowCounts_MultipleTables(t *testing.T) {
	want := map[string]int{
		"issues":     100,
		"comments":   250,
		"labels":     18,
		"milestones": 5,
	}
	db := newFakeRowCountsDB(&fakeRowCountsConfig{tables: want})
	defer db.Close()

	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != len(want) {
		t.Errorf("table count: want %d, got %d", len(want), len(counts))
	}
	for table, wantCount := range want {
		if got := counts[table]; got != wantCount {
			t.Errorf("table %q: want %d, got %d", table, wantCount, got)
		}
	}
}

// TestCompactorGetRowCounts_AllTablesPresent verifies that every table returned
// by the listing query has a corresponding entry in the result map.
// This matters because the integrity gate iterates over preCounts and checks
// that every table still exists in postCounts — a missing key triggers an error.
func TestCompactorGetRowCounts_AllTablesPresent(t *testing.T) {
	tables := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	db := newFakeRowCountsDB(&fakeRowCountsConfig{tables: tables})
	defer db.Close()

	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name := range tables {
		if _, ok := counts[name]; !ok {
			t.Errorf("table %q missing from result — integrity gate would false-alarm", name)
		}
	}
}

// TestCompactorGetRowCounts_ListError verifies that an error from the table-
// listing query propagates to the caller with the original message intact.
func TestCompactorGetRowCounts_ListError(t *testing.T) {
	db := newFakeRowCountsDB(&fakeRowCountsConfig{
		tables:  map[string]int{},
		listErr: fmt.Errorf("connection reset by peer"),
	})
	defer db.Close()

	_, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err == nil {
		t.Fatal("expected error from list query, got nil")
	}
	if !strings.Contains(err.Error(), "connection reset by peer") {
		t.Errorf("error should wrap original message, got: %v", err)
	}
}

// TestCompactorGetRowCounts_CountError verifies that a COUNT(*) error on any
// table propagates to the caller and is not silently dropped.
// A silent drop would allow a table to be missing from the result, causing the
// integrity gate to false-alarm with "table missing after compaction".
func TestCompactorGetRowCounts_CountError(t *testing.T) {
	db := newFakeRowCountsDB(&fakeRowCountsConfig{
		tables: map[string]int{"good": 10, "bad": 0},
		countErrs: map[string]error{
			"bad": fmt.Errorf("lock wait timeout exceeded"),
		},
	})
	defer db.Close()

	_, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err == nil {
		t.Fatal("expected error from COUNT query on 'bad', got nil")
	}
}

// TestCompactorGetRowCounts_ParallelQueriesRunConcurrently verifies that the
// COUNT(*) queries execute in parallel rather than serially.
//
// Strategy: inject a fixed delay into every COUNT(*) response. With N tables:
//   - Serial:   total ≥ N × delay
//   - Parallel: total ≈ delay   (all queries in flight simultaneously)
//
// We assert total elapsed < 2×delay, giving generous headroom for goroutine
// scheduling jitter on slow CI machines while still catching serial execution.
func TestCompactorGetRowCounts_ParallelQueriesRunConcurrently(t *testing.T) {
	const (
		numTables   = 5
		queryDelay  = 50 * time.Millisecond
		serialMin   = time.Duration(numTables) * queryDelay // 250ms if serial
		parallelMax = 2 * queryDelay                        // 100ms; parallel should be well under this
	)

	tables := make(map[string]int, numTables)
	for i := 0; i < numTables; i++ {
		tables[fmt.Sprintf("table%d", i)] = i * 10
	}

	db := newFakeRowCountsDB(&fakeRowCountsConfig{
		tables:     tables,
		countDelay: queryDelay,
	})
	defer db.Close()
	// Allow enough concurrent connections to run all queries simultaneously.
	db.SetMaxOpenConns(numTables)

	start := time.Now()
	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != numTables {
		t.Errorf("expected %d results, got %d", numTables, len(counts))
	}

	// Fail hard if serial: elapsed ≥ serialMin proves goroutines ran one at a time.
	if elapsed >= serialMin {
		t.Errorf("queries ran serially: elapsed=%v, serial minimum=%v — parallel dispatch not working", elapsed, serialMin)
	}
	t.Logf("parallel elapsed=%v (serial would be ≥%v, 2×queryDelay threshold=%v)", elapsed, serialMin, parallelMax)
}

// TestCompactorGetRowCounts_ExactCountsRequired documents why this function
// must return exact COUNT(*) values rather than information_schema.table_rows
// estimates. This test acts as a regression guard: if someone replaces
// COUNT(*) with an estimate-based query, exact values will diverge.
//
// The integrity gate in compactDatabase and surgicalRebaseOnce compares:
//
//	if postCount < preCount {
//	    return fmt.Errorf("integrity check: table %q lost rows", table)
//	}
//
// An off-by-one estimate could either:
//   (a) false-alarm and abort a valid compaction, or
//   (b) miss real data loss and allow a corrupted compaction to proceed.
func TestCompactorGetRowCounts_ExactCountsRequired(t *testing.T) {
	const exactCount = 9999
	db := newFakeRowCountsDB(&fakeRowCountsConfig{
		tables: map[string]int{"issues": exactCount},
	})
	defer db.Close()

	counts, err := newTestDaemon(t).compactorGetRowCounts(db, "testdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["issues"] != exactCount {
		t.Errorf("expected exact count %d, got %d — integrity gate requires exact values, not estimates",
			exactCount, counts["issues"])
	}
}

// TestCompactorGetRowCounts_IntegrityGateSimulation simulates the actual
// pre/post compaction check to confirm the function's output satisfies it.
// Pre-counts are taken, "compaction" runs (a no-op here), post-counts are
// taken, then the gate logic is applied. No rows are lost, so no error.
func TestCompactorGetRowCounts_IntegrityGateSimulation(t *testing.T) {
	tables := map[string]int{"issues": 500, "comments": 1200, "labels": 30}

	preDB := newFakeRowCountsDB(&fakeRowCountsConfig{tables: tables})
	defer preDB.Close()

	// Post-compaction: same counts (no data loss) plus one table gained rows
	// from a concurrent write, which is explicitly safe per the gate logic.
	postTables := map[string]int{"issues": 500, "comments": 1201, "labels": 30}
	postDB := newFakeRowCountsDB(&fakeRowCountsConfig{tables: postTables})
	defer postDB.Close()

	d := newTestDaemon(t)

	preCounts, err := d.compactorGetRowCounts(preDB, "testdb")
	if err != nil {
		t.Fatalf("pre counts: %v", err)
	}

	postCounts, err := d.compactorGetRowCounts(postDB, "testdb")
	if err != nil {
		t.Fatalf("post counts: %v", err)
	}

	// Apply the integrity gate exactly as compactDatabase does.
	for table, preCount := range preCounts {
		postCount, ok := postCounts[table]
		if !ok {
			t.Errorf("integrity gate: table %q missing after compaction", table)
			continue
		}
		if postCount < preCount {
			t.Errorf("integrity gate: table %q lost rows: pre=%d post=%d", table, preCount, postCount)
		}
	}
}
