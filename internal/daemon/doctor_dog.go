package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Operational constants — timeouts needed to perform checks.
const (
	defaultDoctorDogInterval = 5 * time.Minute
	doctorDogTCPTimeout      = 5 * time.Second
	doctorDogQueryTimeout    = 10 * time.Second
	doctorDogGCTimeout       = 5 * time.Minute
)

// Response thresholds — when to take automated action.
const (
	// doctorDogLatencyAlertMs is the latency threshold (in ms) that triggers
	// an escalation to the Mayor. 5 seconds indicates severe degradation.
	doctorDogLatencyAlertMs = 5000.0

	// doctorDogOrphanAlertCount is the database count threshold that triggers
	// a janitor run. More than 10 is concerning; > 20 triggers cleanup.
	doctorDogOrphanAlertCount = 20

	// doctorDogBackupStaleSeconds is the backup age (in seconds) that triggers
	// an immediate backup sync. 1 hour = 3600 seconds.
	doctorDogBackupStaleSeconds = 3600.0

	// doctorDogActionCooldown prevents repeated actions within this window.
	// Each action type has its own cooldown tracker.
	doctorDogActionCooldown = 10 * time.Minute
)

// DoctorDogConfig holds configuration for the doctor_dog patrol.
type DoctorDogConfig struct {
	// Enabled controls whether the doctor dog runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to run, as a string (e.g., "5m").
	IntervalStr string `json:"interval,omitempty"`

	// Databases lists the expected production databases.
	// If empty, uses the default set.
	Databases []string `json:"databases,omitempty"`
}

// --- Report types: structured data for agent consumption ---

// DoctorDogReport is the complete output of a doctor dog health check cycle.
// Agents (Deacon/Mayor) consume this to make escalation decisions.
type DoctorDogReport struct {
	Timestamp      time.Time                  `json:"timestamp"`
	Host           string                     `json:"host"`
	Port           int                        `json:"port"`
	TCPReachable   bool                       `json:"tcp_reachable"`
	Latency        *DoctorDogLatencyReport    `json:"latency,omitempty"`
	Databases      *DoctorDogDatabasesReport  `json:"databases,omitempty"`
	GC             []DoctorDogGCReport        `json:"gc,omitempty"`
	Zombies        []DoctorDogZombieReport    `json:"zombies,omitempty"`
	BackupAge      *DoctorDogBackupReport     `json:"backup_age,omitempty"`
	JsonlBackupAge *DoctorDogBackupReport     `json:"jsonl_backup_age,omitempty"`
	DiskUsage      []DoctorDogDiskReport      `json:"disk_usage,omitempty"`
}

// DoctorDogLatencyReport records SELECT 1 latency.
type DoctorDogLatencyReport struct {
	DurationMs float64 `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

// DoctorDogDatabasesReport records the databases found via SHOW DATABASES.
type DoctorDogDatabasesReport struct {
	Names []string `json:"names"`
	Count int      `json:"count"`
	Error string   `json:"error,omitempty"`
}

// DoctorDogGCReport records the result of dolt gc on one database.
type DoctorDogGCReport struct {
	Database   string  `json:"database"`
	DurationMs float64 `json:"duration_ms"`
	Success    bool    `json:"success"`
	TimedOut   bool    `json:"timed_out,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// DoctorDogZombieReport records a detected zombie dolt sql-server process.
type DoctorDogZombieReport struct {
	PID     int    `json:"pid"`
	Cmdline string `json:"cmdline"`
}

// DoctorDogBackupReport records backup age data.
type DoctorDogBackupReport struct {
	AgeSeconds float64 `json:"age_seconds"`
	Error      string  `json:"error,omitempty"`
	Missing    bool    `json:"missing,omitempty"`
}

// DoctorDogDiskReport records disk usage for one database.
type DoctorDogDiskReport struct {
	Database  string `json:"database"`
	SizeBytes int64  `json:"size_bytes"`
	SizeMB    int64  `json:"size_mb"`
}

// doctorDogInterval returns the configured interval, or the default (5m).
func doctorDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.DoctorDog != nil {
		if config.Patrols.DoctorDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.DoctorDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultDoctorDogInterval
}

// doctorDogDatabases returns the list of production databases for gc.
func doctorDogDatabases(config *DaemonPatrolConfig) []string {
	if config != nil && config.Patrols != nil && config.Patrols.DoctorDog != nil {
		if len(config.Patrols.DoctorDog.Databases) > 0 {
			return config.Patrols.DoctorDog.Databases
		}
	}
	return []string{"hq", "beads", "gastown", "sky", "wyvern", "beads_hop"}
}

// runDoctorDog performs all health checks, writes a structured report, and
// takes automated response actions when thresholds are exceeded.
// Actions: restart server if unreachable, trigger janitor if orphans > 20,
// trigger backup if stale > 1hr, escalate to Mayor if latency > 5s.
func (d *Daemon) runDoctorDog() {
	if !IsPatrolEnabled(d.patrolConfig, "doctor_dog") {
		return
	}

	d.logger.Printf("doctor_dog: starting health check cycle")

	port := d.doltServerPort()
	host := "127.0.0.1"

	report := &DoctorDogReport{
		Timestamp: time.Now(),
		Host:      host,
		Port:      port,
	}

	// 1. TCP connectivity check
	report.TCPReachable = d.doctorDogTCPCheck(host, port)

	if report.TCPReachable {
		// 2. SELECT 1 latency (only if server is reachable)
		report.Latency = d.doctorDogLatencyReport(host, port)

		// 3. SHOW DATABASES (only if server is reachable)
		report.Databases = d.doctorDogDatabasesReport(host, port)
	}

	// 4. Dolt GC on each production database (filesystem-based)
	report.GC = d.doctorDogRunGC()

	// 5. Zombie server detection
	expectedPorts := []int{port}
	if d.doltTestServer != nil && d.doltTestServer.IsEnabled() {
		expectedPorts = append(expectedPorts, d.doltTestServer.config.Port)
	}
	report.Zombies = d.doctorDogZombieReport(expectedPorts)

	// 6. Backup staleness (Dolt filesystem)
	report.BackupAge = d.doctorDogBackupAgeReport()

	// 6b. JSONL git backup freshness
	report.JsonlBackupAge = d.doctorDogJsonlBackupAgeReport()

	// 7. Disk usage per DB
	report.DiskUsage = d.doctorDogDiskUsageReport()

	// Write structured report for agent consumption
	d.writeDoctorDogReport(report)

	// Evaluate report and take automated response actions
	d.doctorDogRespond(report)

	d.logger.Printf("doctor_dog: health check cycle complete")
}

// doctorDogTCPCheck performs a TCP connection check to the Dolt server.
// Returns true if connection succeeds.
func (d *Daemon) doctorDogTCPCheck(host string, port int) bool {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, doctorDogTCPTimeout)
	if err != nil {
		d.logger.Printf("doctor_dog: TCP check failed: %v", err)
		return false
	}
	conn.Close()
	return true
}

// doctorDogLatencyReport runs SELECT 1 and returns the latency measurement.
func (d *Daemon) doctorDogLatencyReport(host string, port int) *DoctorDogLatencyReport {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/?timeout=5s&readTimeout=10s", host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return &DoctorDogLatencyReport{Error: fmt.Sprintf("open failed: %v", err)}
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), doctorDogQueryTimeout)
	defer cancel()

	start := time.Now()
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return &DoctorDogLatencyReport{Error: fmt.Sprintf("query failed: %v", err)}
	}
	latency := time.Since(start)

	d.logger.Printf("doctor_dog: latency: %v", latency)
	return &DoctorDogLatencyReport{DurationMs: float64(latency.Microseconds()) / 1000.0}
}

// doctorDogDatabasesReport runs SHOW DATABASES and returns the list.
func (d *Daemon) doctorDogDatabasesReport(host string, port int) *DoctorDogDatabasesReport {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/?timeout=5s&readTimeout=10s", host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return &DoctorDogDatabasesReport{Error: fmt.Sprintf("open failed: %v", err)}
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), doctorDogQueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return &DoctorDogDatabasesReport{Error: fmt.Sprintf("query failed: %v", err)}
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		// Skip Dolt internal databases
		if name == "information_schema" || name == "mysql" {
			continue
		}
		databases = append(databases, name)
	}

	d.logger.Printf("doctor_dog: databases: %d found", len(databases))
	return &DoctorDogDatabasesReport{Names: databases, Count: len(databases)}
}

// doctorDogRunGC runs dolt gc on each production database from the filesystem.
// GC is maintenance, not a judgment call — the doctor runs it and reports results.
func (d *Daemon) doctorDogRunGC() []DoctorDogGCReport {
	var dataDir string
	if d.doltServer != nil && d.doltServer.IsEnabled() && d.doltServer.config.DataDir != "" {
		dataDir = d.doltServer.config.DataDir
	} else {
		dataDir = filepath.Join(d.config.TownRoot, ".dolt-data")
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		d.logger.Printf("doctor_dog: gc: data dir %s does not exist, skipping", dataDir)
		return nil
	}

	databases := doctorDogDatabases(d.patrolConfig)
	d.logger.Printf("doctor_dog: gc: running on %d databases", len(databases))

	var results []DoctorDogGCReport
	for _, dbName := range databases {
		dbDir := filepath.Join(dataDir, dbName)
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			d.logger.Printf("doctor_dog: gc: %s: directory not found, skipping", dbName)
			continue
		}

		results = append(results, d.doctorDogGCDatabase(dbDir, dbName))
	}
	return results
}

// doctorDogGCDatabase runs dolt gc on a single database directory and reports result.
func (d *Daemon) doctorDogGCDatabase(dbDir, dbName string) DoctorDogGCReport {
	ctx, cancel := context.WithTimeout(context.Background(), doctorDogGCTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "dolt", "gc")
	cmd.Dir = dbDir

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	durationMs := float64(elapsed.Microseconds()) / 1000.0

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			d.logger.Printf("doctor_dog: gc: %s: TIMEOUT after %v", dbName, elapsed)
			return DoctorDogGCReport{Database: dbName, DurationMs: durationMs, TimedOut: true, Error: "timeout"}
		}
		errMsg := strings.TrimSpace(string(output))
		d.logger.Printf("doctor_dog: gc: %s: failed after %v: %v (%s)", dbName, elapsed, err, errMsg)
		return DoctorDogGCReport{Database: dbName, DurationMs: durationMs, Error: fmt.Sprintf("%v: %s", err, errMsg)}
	}

	d.logger.Printf("doctor_dog: gc: %s: completed in %v", dbName, elapsed)
	return DoctorDogGCReport{Database: dbName, DurationMs: durationMs, Success: true}
}

// doctorDogZombieReport scans for dolt sql-server processes NOT on any expected port.
// Reports findings without taking action — agents decide what to do.
func (d *Daemon) doctorDogZombieReport(expectedPorts []int) []DoctorDogZombieReport {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find all dolt sql-server processes
	cmd := exec.CommandContext(ctx, "pgrep", "-f", "dolt sql-server")
	output, err := cmd.Output()
	if err != nil {
		// pgrep returns exit 1 if no matches — that's fine
		d.logger.Printf("doctor_dog: zombie check: no dolt sql-server processes found")
		return nil
	}

	// Build a set of expected port strings for fast lookup
	expectedPortStrs := make(map[string]bool, len(expectedPorts))
	for _, p := range expectedPorts {
		expectedPortStrs[strconv.Itoa(p)] = true
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	var zombies []DoctorDogZombieReport

	for _, pidStr := range pids {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Check if this process is using an expected port
		cmdlineCtx, cmdlineCancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmdlineCmd := exec.CommandContext(cmdlineCtx, "ps", "-p", pidStr, "-o", "command=")
		cmdlineOutput, cmdlineErr := cmdlineCmd.Output()
		cmdlineCancel()

		if cmdlineErr != nil {
			continue
		}

		cmdline := strings.TrimSpace(string(cmdlineOutput))
		if !strings.Contains(cmdline, "dolt") || !strings.Contains(cmdline, "sql-server") {
			continue
		}

		// Check if this is on any expected port
		isExpected := false
		for portStr := range expectedPortStrs {
			if strings.Contains(cmdline, "--port="+portStr) ||
				strings.Contains(cmdline, "--port "+portStr) ||
				strings.Contains(cmdline, "-p "+portStr) ||
				strings.Contains(cmdline, "-p="+portStr) {
				isExpected = true
				break
			}
		}
		if isExpected {
			continue
		}

		// If no port specified explicitly, could be expected server using default config.
		if !strings.Contains(cmdline, "--port") && !strings.Contains(cmdline, "-p ") {
			continue
		}

		zombies = append(zombies, DoctorDogZombieReport{PID: pid, Cmdline: cmdline})
	}

	if len(zombies) > 0 {
		d.logger.Printf("doctor_dog: zombie check: found %d zombie(s)", len(zombies))
	} else {
		d.logger.Printf("doctor_dog: zombie check: no zombie processes found")
	}
	return zombies
}

// doctorDogBackupAgeReport checks backup age and returns the data.
func (d *Daemon) doctorDogBackupAgeReport() *DoctorDogBackupReport {
	backupDir := filepath.Join(d.config.TownRoot, ".dolt-backup")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		d.logger.Printf("doctor_dog: backup check: %s does not exist, skipping", backupDir)
		return nil
	}

	var newest time.Time
	err := filepath.Walk(backupDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !info.IsDir() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return &DoctorDogBackupReport{Error: fmt.Sprintf("walk error: %v", err)}
	}
	if newest.IsZero() {
		d.logger.Printf("doctor_dog: backup check: no files found in %s", backupDir)
		return &DoctorDogBackupReport{Missing: true}
	}

	age := time.Since(newest)
	d.logger.Printf("doctor_dog: backup check: newest file is %v old", age.Round(time.Second))
	return &DoctorDogBackupReport{AgeSeconds: age.Seconds()}
}

// doctorDogJsonlBackupAgeReport checks JSONL git backup freshness and returns the data.
func (d *Daemon) doctorDogJsonlBackupAgeReport() *DoctorDogBackupReport {
	// Determine the JSONL git repo path from config or default.
	var gitRepo string
	if d.patrolConfig != nil && d.patrolConfig.Patrols != nil && d.patrolConfig.Patrols.JsonlGitBackup != nil {
		gitRepo = d.patrolConfig.Patrols.JsonlGitBackup.GitRepo
	}
	if gitRepo == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return &DoctorDogBackupReport{Error: fmt.Sprintf("cannot determine home dir: %v", err)}
		}
		gitRepo = filepath.Join(homeDir, ".dolt-archive", "git")
	}

	if _, err := os.Stat(filepath.Join(gitRepo, ".git")); os.IsNotExist(err) {
		d.logger.Printf("doctor_dog: jsonl backup check: %s not a git repo, skipping", gitRepo)
		return nil
	}

	// Get the timestamp of the latest commit.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", gitRepo, "log", "-1", "--format=%ci")
	output, err := cmd.Output()
	if err != nil {
		return &DoctorDogBackupReport{Error: fmt.Sprintf("git log failed: %v", err)}
	}

	commitTimeStr := strings.TrimSpace(string(output))
	if commitTimeStr == "" {
		d.logger.Printf("doctor_dog: jsonl backup check: no commits in %s", gitRepo)
		return &DoctorDogBackupReport{Missing: true}
	}

	commitTime, err := time.Parse("2006-01-02 15:04:05 -0700", commitTimeStr)
	if err != nil {
		return &DoctorDogBackupReport{Error: fmt.Sprintf("cannot parse commit time %q: %v", commitTimeStr, err)}
	}

	age := time.Since(commitTime)
	d.logger.Printf("doctor_dog: jsonl backup check: last commit %v ago", age.Round(time.Second))
	return &DoctorDogBackupReport{AgeSeconds: age.Seconds()}
}

// doctorDogDiskUsageReport reports disk usage per database directory.
func (d *Daemon) doctorDogDiskUsageReport() []DoctorDogDiskReport {
	var dataDir string
	if d.doltServer != nil && d.doltServer.IsEnabled() && d.doltServer.config.DataDir != "" {
		dataDir = d.doltServer.config.DataDir
	} else {
		dataDir = filepath.Join(d.config.TownRoot, ".dolt-data")
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil
	}

	databases := doctorDogDatabases(d.patrolConfig)
	var results []DoctorDogDiskReport
	for _, dbName := range databases {
		dbDir := filepath.Join(dataDir, dbName)
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			continue
		}

		size, err := dirSize(dbDir)
		if err != nil {
			d.logger.Printf("doctor_dog: disk check: %s: error calculating size: %v", dbName, err)
			continue
		}

		sizeMB := size / (1024 * 1024)
		d.logger.Printf("doctor_dog: disk check: %s: %dMB", dbName, sizeMB)
		results = append(results, DoctorDogDiskReport{
			Database:  dbName,
			SizeBytes: size,
			SizeMB:    sizeMB,
		})
	}
	return results
}

// dirSize calculates the total size of files in a directory recursively.
func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// doctorDogRespond evaluates the health report and takes automated actions:
// - Restart server if TCP unreachable
// - Trigger janitor if database count > 20 (orphan accumulation)
// - Trigger backup if backup stale > 1 hour
// - Escalate to Mayor if query latency > 5 seconds
// Each action has a cooldown to prevent action storms.
func (d *Daemon) doctorDogRespond(report *DoctorDogReport) {
	now := time.Now()

	// Action 1: Restart server if unreachable
	if !report.TCPReachable {
		if now.Sub(d.lastDoctorRestart) >= doctorDogActionCooldown {
			d.lastDoctorRestart = now
			d.logger.Printf("doctor_dog: ACTION: server unreachable, triggering restart")
			d.ensureDoltServerRunning()
		} else {
			d.logger.Printf("doctor_dog: server unreachable but restart cooldown active")
		}
	}

	// Action 2: Escalate to Mayor if latency > 5s
	if report.Latency != nil && report.Latency.Error == "" && report.Latency.DurationMs > doctorDogLatencyAlertMs {
		if now.Sub(d.lastDoctorEscalate) >= doctorDogActionCooldown {
			d.lastDoctorEscalate = now
			d.logger.Printf("doctor_dog: ACTION: latency %.0fms > %.0fms threshold, escalating",
				report.Latency.DurationMs, doctorDogLatencyAlertMs)
			d.escalate("doctor_dog", fmt.Sprintf("Dolt query latency %.0fms exceeds %ds threshold",
				report.Latency.DurationMs, int(doctorDogLatencyAlertMs/1000)))
		}
	}

	// Action 3: Trigger janitor if orphan databases > 20
	if report.Databases != nil && report.Databases.Error == "" && report.Databases.Count > doctorDogOrphanAlertCount {
		if now.Sub(d.lastDoctorJanitor) >= doctorDogActionCooldown {
			d.lastDoctorJanitor = now
			d.logger.Printf("doctor_dog: ACTION: %d databases (> %d threshold), triggering janitor",
				report.Databases.Count, doctorDogOrphanAlertCount)
			go d.runJanitorDog()
		}
	}

	// Action 4: Trigger backup if stale > 1 hour
	if report.BackupAge != nil && report.BackupAge.Error == "" && !report.BackupAge.Missing &&
		report.BackupAge.AgeSeconds > doctorDogBackupStaleSeconds {
		if now.Sub(d.lastDoctorBackup) >= doctorDogActionCooldown {
			d.lastDoctorBackup = now
			d.logger.Printf("doctor_dog: ACTION: backup %.0fs stale (> %.0fs threshold), triggering backup",
				report.BackupAge.AgeSeconds, doctorDogBackupStaleSeconds)
			go d.syncDoltBackups()
		}
	}
}

// writeDoctorDogReport writes the report as JSON to a well-known location.
func (d *Daemon) writeDoctorDogReport(report *DoctorDogReport) {
	reportPath := filepath.Join(d.config.TownRoot, ".doctor-dog-report.json")

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		d.logger.Printf("doctor_dog: failed to marshal report: %v", err)
		return
	}

	if err := os.WriteFile(reportPath, data, 0644); err != nil {
		d.logger.Printf("doctor_dog: failed to write report to %s: %v", reportPath, err)
	}
}
