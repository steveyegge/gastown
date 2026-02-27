package daemon

import (
	"context"
	"database/sql"
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

const (
	defaultDoctorDogInterval = 5 * time.Minute
	doctorDogTCPTimeout      = 5 * time.Second
	doctorDogQueryTimeout    = 10 * time.Second
	doctorDogGCTimeout       = 5 * time.Minute
	doctorDogLatencyThreshold = 2 * time.Second
	doctorDogDefaultMaxDBCount = 6
	doctorDogBackupStaleAge  = 30 * time.Minute
	doctorDogDBSizeLimit     = 200 * 1024 * 1024 // 200MB
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

	// MaxDBCount is the maximum expected database count from SHOW DATABASES.
	// Escalate if actual count exceeds this.
	MaxDBCount int `json:"max_db_count,omitempty"`
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

// doctorDogMaxDBCount returns the max expected DB count.
func doctorDogMaxDBCount(config *DaemonPatrolConfig) int {
	if config != nil && config.Patrols != nil && config.Patrols.DoctorDog != nil {
		if config.Patrols.DoctorDog.MaxDBCount > 0 {
			return config.Patrols.DoctorDog.MaxDBCount
		}
	}
	return doctorDogDefaultMaxDBCount
}

// runDoctorDog performs all health checks for the Doctor Dog patrol.
// Non-fatal: errors are logged and escalated but don't stop the daemon.
func (d *Daemon) runDoctorDog() {
	if !IsPatrolEnabled(d.patrolConfig, "doctor_dog") {
		return
	}

	d.logger.Printf("doctor_dog: starting health check cycle")

	port := d.doltServerPort()
	host := "127.0.0.1"

	// 1. TCP connectivity check
	if !d.doctorDogTCPCheck(host, port) {
		// Server unreachable — attempt restart
		d.logger.Printf("doctor_dog: server unreachable on %s:%d, attempting restart", host, port)
		d.escalate("doctor_dog", fmt.Sprintf("Dolt server unreachable on %s:%d, attempting restart", host, port))
		d.doctorDogRestartServer()
		return // Skip remaining checks — server just restarted
	}

	// 2. SELECT 1 latency check
	d.doctorDogLatencyCheck(host, port)

	// 3. SHOW DATABASES count check
	d.doctorDogDatabaseCountCheck(host, port)

	// 4. Dolt GC on each production database
	d.doctorDogRunGC()

	// 5. Zombie server detection (exclude both prod and test server ports)
	expectedPorts := []int{port}
	if d.doltTestServer != nil && d.doltTestServer.IsEnabled() {
		expectedPorts = append(expectedPorts, d.doltTestServer.config.Port)
	}
	d.doctorDogZombieCheck(expectedPorts)

	// 6. Backup staleness check
	d.doctorDogBackupStalenessCheck()

	// 7. Disk usage per DB
	d.doctorDogDiskUsageCheck()

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

// doctorDogLatencyCheck runs SELECT 1 and checks response latency.
func (d *Daemon) doctorDogLatencyCheck(host string, port int) {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/?timeout=5s&readTimeout=10s", host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		d.logger.Printf("doctor_dog: latency check: open failed: %v", err)
		d.escalate("doctor_dog", fmt.Sprintf("Cannot open connection for latency check: %v", err))
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), doctorDogQueryTimeout)
	defer cancel()

	start := time.Now()
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		d.logger.Printf("doctor_dog: latency check: query failed: %v", err)
		d.escalate("doctor_dog", fmt.Sprintf("SELECT 1 failed: %v", err))
		return
	}
	latency := time.Since(start)

	if latency > doctorDogLatencyThreshold {
		d.logger.Printf("doctor_dog: latency check: %v exceeds threshold %v", latency, doctorDogLatencyThreshold)
		d.escalate("doctor_dog", fmt.Sprintf("SELECT 1 latency %v exceeds %v threshold", latency, doctorDogLatencyThreshold))
	} else {
		d.logger.Printf("doctor_dog: latency check: %v (OK)", latency)
	}
}

// doctorDogDatabaseCountCheck runs SHOW DATABASES and checks the count.
func (d *Daemon) doctorDogDatabaseCountCheck(host string, port int) {
	dsn := fmt.Sprintf("root@tcp(%s:%d)/?timeout=5s&readTimeout=10s", host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		d.logger.Printf("doctor_dog: db count check: open failed: %v", err)
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), doctorDogQueryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		d.logger.Printf("doctor_dog: db count check: query failed: %v", err)
		d.escalate("doctor_dog", fmt.Sprintf("SHOW DATABASES failed: %v", err))
		return
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

	maxCount := doctorDogMaxDBCount(d.patrolConfig)
	if len(databases) > maxCount {
		d.logger.Printf("doctor_dog: db count check: %d databases (max %d): %v", len(databases), maxCount, databases)
		d.escalate("doctor_dog", fmt.Sprintf("Database count %d exceeds expected max %d. DBs: %v", len(databases), maxCount, databases))
	} else {
		d.logger.Printf("doctor_dog: db count check: %d databases (OK, max %d)", len(databases), maxCount)
	}
}

// doctorDogRunGC runs dolt gc on each production database from the filesystem.
// Runs one DB at a time with a timeout per DB.
func (d *Daemon) doctorDogRunGC() {
	var dataDir string
	if d.doltServer != nil && d.doltServer.IsEnabled() && d.doltServer.config.DataDir != "" {
		dataDir = d.doltServer.config.DataDir
	} else {
		dataDir = filepath.Join(d.config.TownRoot, ".dolt-data")
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		d.logger.Printf("doctor_dog: gc: data dir %s does not exist, skipping", dataDir)
		return
	}

	databases := doctorDogDatabases(d.patrolConfig)
	d.logger.Printf("doctor_dog: gc: running on %d databases", len(databases))

	for _, dbName := range databases {
		dbDir := filepath.Join(dataDir, dbName)
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			d.logger.Printf("doctor_dog: gc: %s: directory not found, skipping", dbName)
			continue
		}

		d.doctorDogGCDatabase(dbDir, dbName)
	}
}

// doctorDogGCDatabase runs dolt gc on a single database directory.
func (d *Daemon) doctorDogGCDatabase(dbDir, dbName string) {
	ctx, cancel := context.WithTimeout(context.Background(), doctorDogGCTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "dolt", "gc")
	cmd.Dir = dbDir

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			d.logger.Printf("doctor_dog: gc: %s: TIMEOUT after %v", dbName, elapsed)
			d.escalate("doctor_dog", fmt.Sprintf("dolt gc timed out on %s after %v", dbName, elapsed))
		} else {
			d.logger.Printf("doctor_dog: gc: %s: failed after %v: %v (%s)", dbName, elapsed, err, strings.TrimSpace(string(output)))
			d.escalate("doctor_dog", fmt.Sprintf("dolt gc failed on %s: %v", dbName, err))
		}
		return
	}

	d.logger.Printf("doctor_dog: gc: %s: completed in %v", dbName, elapsed)
}

// doctorDogZombieCheck scans for dolt sql-server processes NOT on any of the expected ports.
func (d *Daemon) doctorDogZombieCheck(expectedPorts []int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find all dolt sql-server processes
	cmd := exec.CommandContext(ctx, "pgrep", "-f", "dolt sql-server")
	output, err := cmd.Output()
	if err != nil {
		// pgrep returns exit 1 if no matches — that's fine
		d.logger.Printf("doctor_dog: zombie check: no dolt sql-server processes found")
		return
	}

	// Build a set of expected port strings for fast lookup
	expectedPortStrs := make(map[string]bool, len(expectedPorts))
	for _, p := range expectedPorts {
		expectedPortStrs[strconv.Itoa(p)] = true
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	var zombies []int

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

		// If no port specified explicitly and it's the only process, it could
		// be using the default. But to be safe, only kill confirmed zombies
		// that specify a different port.
		if !strings.Contains(cmdline, "--port") && !strings.Contains(cmdline, "-p ") {
			// No explicit port — could be expected server using default config.
			// Don't kill unless we know it's a zombie.
			continue
		}

		zombies = append(zombies, pid)
	}

	if len(zombies) == 0 {
		d.logger.Printf("doctor_dog: zombie check: no zombie processes found")
		return
	}

	d.logger.Printf("doctor_dog: zombie check: found %d zombie(s): %v", len(zombies), zombies)
	d.escalate("doctor_dog", fmt.Sprintf("Found %d zombie dolt sql-server process(es): %v — killing", len(zombies), zombies))

	for _, pid := range zombies {
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		killCmd := exec.CommandContext(killCtx, "kill", strconv.Itoa(pid))
		if killErr := killCmd.Run(); killErr != nil {
			d.logger.Printf("doctor_dog: zombie check: failed to kill PID %d: %v", pid, killErr)
		} else {
			d.logger.Printf("doctor_dog: zombie check: killed zombie PID %d", pid)
		}
		killCancel()
	}
}

// doctorDogBackupStalenessCheck checks if backups are stale by finding the most
// recently modified file across all backup subdirectories. Parent directory mtime
// doesn't update when files inside subdirectories change, so we walk the tree.
func (d *Daemon) doctorDogBackupStalenessCheck() {
	backupDir := filepath.Join(d.config.TownRoot, ".dolt-backup")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		d.logger.Printf("doctor_dog: backup check: %s does not exist, skipping", backupDir)
		return
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
		d.logger.Printf("doctor_dog: backup check: walk error: %v", err)
		return
	}
	if newest.IsZero() {
		d.logger.Printf("doctor_dog: backup check: no files found in %s", backupDir)
		d.escalate("doctor_dog", fmt.Sprintf("Backup directory %s contains no files", backupDir))
		return
	}

	age := time.Since(newest)
	if age > doctorDogBackupStaleAge {
		d.logger.Printf("doctor_dog: backup check: newest file is %v old (threshold %v)", age.Round(time.Second), doctorDogBackupStaleAge)
		d.escalate("doctor_dog", fmt.Sprintf("Backup newest file %v ago (threshold %v)", age.Round(time.Minute), doctorDogBackupStaleAge))
	} else {
		d.logger.Printf("doctor_dog: backup check: newest file is %v old (OK)", age.Round(time.Second))
	}
}

// doctorDogDiskUsageCheck checks disk usage per database directory.
func (d *Daemon) doctorDogDiskUsageCheck() {
	var dataDir string
	if d.doltServer != nil && d.doltServer.IsEnabled() && d.doltServer.config.DataDir != "" {
		dataDir = d.doltServer.config.DataDir
	} else {
		dataDir = filepath.Join(d.config.TownRoot, ".dolt-data")
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return
	}

	databases := doctorDogDatabases(d.patrolConfig)
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
		if size > doctorDogDBSizeLimit {
			d.logger.Printf("doctor_dog: disk check: %s: %dMB exceeds %dMB limit", dbName, sizeMB, doctorDogDBSizeLimit/(1024*1024))
			d.escalate("doctor_dog", fmt.Sprintf("Database %s disk usage %dMB exceeds %dMB limit", dbName, sizeMB, doctorDogDBSizeLimit/(1024*1024)))
		} else {
			d.logger.Printf("doctor_dog: disk check: %s: %dMB (OK)", dbName, sizeMB)
		}
	}
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

// doctorDogRestartServer attempts to restart the Dolt server via stop+start.
func (d *Daemon) doctorDogRestartServer() {
	if d.doltServer == nil || !d.doltServer.IsEnabled() {
		d.logger.Printf("doctor_dog: restart: dolt server not configured, skipping")
		return
	}

	d.logger.Printf("doctor_dog: restart: stopping dolt server...")
	if err := d.doltServer.Stop(); err != nil {
		d.logger.Printf("doctor_dog: restart: stop returned error: %v", err)
	}

	// Small pause to let the port free up
	time.Sleep(2 * time.Second)

	d.logger.Printf("doctor_dog: restart: starting dolt server...")
	if err := d.doltServer.Start(); err != nil {
		d.logger.Printf("doctor_dog: restart: failed to start: %v", err)
		d.escalate("doctor_dog", fmt.Sprintf("Failed to restart Dolt server: %v", err))
	} else {
		d.logger.Printf("doctor_dog: restart: server restarted successfully")
	}
}
