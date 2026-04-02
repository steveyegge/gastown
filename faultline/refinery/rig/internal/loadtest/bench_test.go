package loadtest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/ingest"
	"github.com/outdoorsea/faultline/internal/server"
	"log/slog"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_loadtest"
	}

	dolt, err := db.Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() { _ = dolt.Close() })

	auth, _ := ingest.NewProjectAuth([]string{"1:loadtest_key"})
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	handler := &ingest.Handler{
		DB:   dolt,
		Auth: auth,
		Log:  log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/{project_id}/envelope/", handler.HandleEnvelope)
	mux.HandleFunc("POST /api/{project_id}/store/", handler.HandleStore)

	return httptest.NewServer(mux)
}

func makeEnvelope(eventNum int) []byte {
	eventID := fmt.Sprintf("%032x", eventNum)
	header := fmt.Sprintf(`{"event_id":"%s","sent_at":"%s"}`, eventID, time.Now().UTC().Format(time.RFC3339))
	itemHeader := `{"type":"event"}`
	payload := fmt.Sprintf(`{
		"event_id":"%s",
		"timestamp":%f,
		"platform":"go",
		"level":"error",
		"message":"loadtest error %d",
		"exception":{"values":[{
			"type":"RuntimeError",
			"value":"loadtest error %d",
			"stacktrace":{"frames":[
				{"module":"main","function":"run"},
				{"module":"app","function":"start"},
				{"module":"app.core","function":"process_%d"},
				{"module":"app.core","function":"validate"},
				{"module":"app.core","function":"execute"}
			]}
		}]}
	}`, eventID, float64(time.Now().Unix()), eventNum, eventNum, eventNum%50)

	return []byte(header + "\n" + itemHeader + "\n" + payload + "\n")
}

func postEnvelope(client *http.Client, url string, body []byte, gzipped bool) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	req.Header.Set("X-Sentry-Auth", "Sentry sentry_key=loadtest_key, sentry_version=7")
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	return client.Do(req)
}

func makeGzipEnvelope(eventNum int) []byte {
	raw := makeEnvelope(eventNum)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestLoadSequential measures single-threaded throughput.
func TestLoadSequential(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	srv := setupTestServer(t)
	defer srv.Close()
	client := srv.Client()

	n := 500
	start := time.Now()
	var errors int

	for i := 0; i < n; i++ {
		env := makeEnvelope(i)
		resp, err := postEnvelope(client, srv.URL+"/api/1/envelope/", env, false)
		if err != nil {
			errors++
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			errors++
		}
	}

	elapsed := time.Since(start)
	rate := float64(n) / elapsed.Seconds()
	t.Logf("Sequential: %d events in %s (%.1f ev/s, %d errors)", n, elapsed, rate, errors)
}

// TestLoadConcurrent measures throughput under concurrent load.
func TestLoadConcurrent(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	srv := setupTestServer(t)
	defer srv.Close()

	for _, concurrency := range []int{1, 5, 10, 25, 50} {
		t.Run(fmt.Sprintf("c%d", concurrency), func(t *testing.T) {
			n := 500
			var completed atomic.Int64
			var errors atomic.Int64
			var totalLatency atomic.Int64

			start := time.Now()
			var wg sync.WaitGroup

			for c := 0; c < concurrency; c++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					client := &http.Client{Timeout: 30 * time.Second}
					perWorker := n / concurrency

					for i := 0; i < perWorker; i++ {
						eventNum := workerID*perWorker + i + 100000 // offset to avoid dupes
						env := makeEnvelope(eventNum)

						reqStart := time.Now()
						resp, err := postEnvelope(client, srv.URL+"/api/1/envelope/", env, false)
						latency := time.Since(reqStart)
						totalLatency.Add(latency.Microseconds())

						if err != nil {
							errors.Add(1)
							continue
						}
						_ = resp.Body.Close()
						if resp.StatusCode != 200 {
							errors.Add(1)
						} else {
							completed.Add(1)
						}
					}
				}(c)
			}

			wg.Wait()
			elapsed := time.Since(start)
			rate := float64(completed.Load()) / elapsed.Seconds()
			avgLatency := time.Duration(0)
			if completed.Load() > 0 {
				avgLatency = time.Duration(totalLatency.Load()/completed.Load()) * time.Microsecond
			}

			t.Logf("Concurrency=%d: %d/%d events in %s (%.1f ev/s, avg latency %s, %d errors)",
				concurrency, completed.Load(), n, elapsed, rate, avgLatency, errors.Load())
		})
	}
}

// TestLoadSustained runs a sustained load for 30 seconds and reports throughput.
func TestLoadSustained(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	srv := setupTestServer(t)
	defer srv.Close()

	duration := 30 * time.Second
	concurrency := 10
	var completed atomic.Int64
	var errors atomic.Int64
	var counter atomic.Int64

	start := time.Now()
	deadline := start.Add(duration)
	var wg sync.WaitGroup

	for c := 0; c < concurrency; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 30 * time.Second}

			for time.Now().Before(deadline) {
				eventNum := int(counter.Add(1)) + 200000
				env := makeEnvelope(eventNum)

				resp, err := postEnvelope(client, srv.URL+"/api/1/envelope/", env, false)
				if err != nil {
					errors.Add(1)
					continue
				}
				_ = resp.Body.Close()
				if resp.StatusCode != 200 {
					errors.Add(1)
				} else {
					completed.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)
	rate := float64(completed.Load()) / elapsed.Seconds()

	t.Logf("Sustained (%s, c=%d): %d events (%.1f ev/s, %d errors)",
		duration, concurrency, completed.Load(), rate, errors.Load())

	// Report issue group count
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_loadtest"
	}
	dolt, err := db.Open(dsn)
	if err == nil {
		defer func() { _ = dolt.Close() }()
		var eventCount, groupCount int
		_ = dolt.QueryRow("SELECT COUNT(*) FROM ft_events").Scan(&eventCount)
		_ = dolt.QueryRow("SELECT COUNT(*) FROM issue_groups").Scan(&groupCount)
		t.Logf("DB state: %d events, %d issue groups", eventCount, groupCount)
	}
}

// TestLoadGzip measures overhead of gzip decompression.
func TestLoadGzip(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	srv := setupTestServer(t)
	defer srv.Close()
	client := srv.Client()

	n := 200
	start := time.Now()
	var errors int

	for i := 0; i < n; i++ {
		env := makeGzipEnvelope(i + 300000)
		resp, err := postEnvelope(client, srv.URL+"/api/1/envelope/", env, true)
		if err != nil {
			errors++
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			errors++
		}
	}

	elapsed := time.Since(start)
	rate := float64(n) / elapsed.Seconds()
	t.Logf("Gzip: %d events in %s (%.1f ev/s, %d errors)", n, elapsed, rate, errors)
}

// TestLoadFingerprinting measures fingerprint distribution.
func TestLoadFingerprinting(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	// Generate 1000 events and count unique fingerprints.
	seen := make(map[string]int)
	for i := 0; i < 1000; i++ {
		env := makeEnvelope(i)
		// Extract just the event payload (3rd line).
		lines := bytes.SplitN(env, []byte("\n"), 4)
		if len(lines) >= 3 {
			fp := ingest.Fingerprint(json.RawMessage(lines[2]))
			seen[fp]++
		}
	}

	t.Logf("Fingerprinting: 1000 events → %d unique groups", len(seen))
	// With eventNum%50 in frame, expect ~50 groups.
	if len(seen) < 40 || len(seen) > 60 {
		t.Logf("WARNING: Expected ~50 groups, got %d", len(seen))
	}
}

// Utility: print server config for manual testing.
func TestPrintServerConfig(t *testing.T) {
	if os.Getenv("FAULTLINE_LOADTEST") == "" {
		t.Skip("Set FAULTLINE_LOADTEST=1 to run load tests")
	}

	_ = server.Config{} // ensure import
	t.Log("Load test configuration:")
	t.Logf("  FAULTLINE_DSN: %s", os.Getenv("FAULTLINE_DSN"))
	t.Logf("  Test creates ~50 unique issue groups from exception frame variation")
	t.Logf("  Events use project_id=1, key=loadtest_key")
}
