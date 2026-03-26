package verify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// Phase identifies when a verification gate runs.
type Phase string

const (
	PhasePreMerge   Phase = Phase(config.MergeQueueGatePhasePreMerge)
	PhasePostSquash Phase = Phase(config.MergeQueueGatePhasePostSquash)
)

// Gate defines a runnable verification command.
type Gate struct {
	Name    string
	Cmd     string
	Timeout time.Duration
	Phase   Phase
}

// Result captures the outcome of a single gate.
type Result struct {
	Name    string
	Success bool
	Error   string
	Output  string
	Elapsed time.Duration
}

// Summary captures the outcome of an entire verification run.
type Summary struct {
	Success bool
	Results []Result
	Error   string
}

// GatesForPhase extracts and normalizes merge-queue gates for the requested phase.
func GatesForPhase(mq *config.MergeQueueConfig, phase Phase) ([]Gate, error) {
	if mq == nil || len(mq.Gates) == 0 {
		return nil, nil
	}

	gates := make([]Gate, 0, len(mq.Gates))
	for name, gateCfg := range mq.Gates {
		if gateCfg == nil {
			return nil, fmt.Errorf("gate %q is null", name)
		}
		gatePhase := Phase(config.MergeQueueGatePhasePreMerge)
		if gateCfg.Phase != "" {
			gatePhase = Phase(gateCfg.Phase)
		}
		if gatePhase != phase {
			continue
		}

		gate := Gate{
			Name:  name,
			Cmd:   gateCfg.Cmd,
			Phase: gatePhase,
		}
		if gateCfg.Timeout != "" {
			timeout, err := time.ParseDuration(gateCfg.Timeout)
			if err != nil {
				return nil, fmt.Errorf("invalid timeout for gate %q: %w", name, err)
			}
			gate.Timeout = timeout
		}
		gates = append(gates, gate)
	}

	sort.Slice(gates, func(i, j int) bool {
		return gates[i].Name < gates[j].Name
	})

	return gates, nil
}

// Run executes the provided gates in the given worktree.
func Run(ctx context.Context, workDir string, gates []Gate, parallel bool, logf func(format string, args ...interface{})) Summary {
	if len(gates) == 0 {
		return Summary{Success: true}
	}

	log := func(format string, args ...interface{}) {
		if logf != nil {
			logf(format, args...)
		}
	}

	results := make([]Result, 0, len(gates))
	if parallel {
		results = make([]Result, len(gates))
		var wg sync.WaitGroup
		for i, gate := range gates {
			wg.Add(1)
			go func(idx int, gate Gate) {
				defer wg.Done()
				log("gate %q: starting (%s)", gate.Name, gate.Cmd)
				results[idx] = runGate(ctx, workDir, gate)
			}(i, gate)
		}
		wg.Wait()
	} else {
		for _, gate := range gates {
			log("gate %q: starting (%s)", gate.Name, gate.Cmd)
			result := runGate(ctx, workDir, gate)
			results = append(results, result)
			if !result.Success {
				break
			}
		}
	}

	var failures []string
	for _, result := range results {
		if result.Success {
			log("gate %q: passed (%v)", result.Name, result.Elapsed.Truncate(time.Millisecond))
			continue
		}
		log("gate %q: FAILED (%v) - %s", result.Name, result.Elapsed.Truncate(time.Millisecond), result.Error)
		failures = append(failures, fmt.Sprintf("%s: %s", result.Name, result.Error))
	}

	if len(failures) > 0 {
		return Summary{
			Success: false,
			Results: results,
			Error:   fmt.Sprintf("quality gates failed: %s", strings.Join(failures, "; ")),
		}
	}

	log("all quality gates passed")
	return Summary{Success: true, Results: results}
}

// FailedGateNames returns the names of failing gates in execution order.
func FailedGateNames(summary Summary) []string {
	var names []string
	for _, result := range summary.Results {
		if !result.Success {
			names = append(names, result.Name)
		}
	}
	return names
}

func runGate(ctx context.Context, workDir string, gate Gate) Result {
	start := time.Now()
	if strings.TrimSpace(gate.Cmd) == "" {
		return Result{
			Name:    gate.Name,
			Success: false,
			Error:   "gate command is empty",
			Elapsed: time.Since(start),
		}
	}

	gateCtx := ctx
	if gate.Timeout > 0 {
		var cancel context.CancelFunc
		gateCtx, cancel = context.WithTimeout(ctx, gate.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(gateCtx, "sh", "-c", gate.Cmd) //nolint:gosec // G204: trusted repo configuration
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "CI=true")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start)
	if err == nil {
		return Result{
			Name:    gate.Name,
			Success: true,
			Elapsed: elapsed,
		}
	}

	output := strings.TrimSpace(strings.Join([]string{
		stdout.String(),
		stderr.String(),
	}, "\n"))
	output = tailOutput(output, 40, 2000)

	errMsg := fmt.Sprintf("%v", err)
	if gateCtx.Err() == context.DeadlineExceeded {
		errMsg = fmt.Sprintf("timed out after %v", gate.Timeout)
	}
	if output != "" {
		errMsg = fmt.Sprintf("%s: %s", errMsg, output)
	}

	return Result{
		Name:    gate.Name,
		Success: false,
		Error:   errMsg,
		Output:  output,
		Elapsed: elapsed,
	}
}

func tailOutput(output string, maxLines, maxBytes int) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	output = strings.Join(lines, "\n")
	if len(output) > maxBytes {
		output = output[len(output)-maxBytes:]
	}
	return strings.TrimSpace(output)
}
