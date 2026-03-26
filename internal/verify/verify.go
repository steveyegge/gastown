package verify

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase identifies where a gate runs in the merge pipeline.
type Phase string

const (
	PhasePreMerge   Phase = "pre-merge"
	PhasePostSquash Phase = "post-squash"
)

// Gate defines a named verification command.
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

// Summary aggregates a verification run.
type Summary struct {
	Success bool
	Results []Result
}

// CommandFunc executes a shell command for a gate.
type CommandFunc func(ctx context.Context, workDir string, env []string, command string) ([]byte, error)

// RunOptions tunes gate execution.
type RunOptions struct {
	Parallel bool
	Env      []string
	Output   io.Writer
	Command  CommandFunc
}

// FilterPhase returns gates matching the given phase.
func FilterPhase(gates []Gate, phase Phase) []Gate {
	var filtered []Gate
	for _, gate := range gates {
		gatePhase := gate.Phase
		if gatePhase == "" {
			gatePhase = PhasePreMerge
		}
		if gatePhase == phase {
			filtered = append(filtered, gate)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	return filtered
}

// RunPhase executes all gates for a phase and returns a summary.
func RunPhase(ctx context.Context, workDir string, gates []Gate, phase Phase, opts RunOptions) Summary {
	filtered := FilterPhase(gates, phase)
	if len(filtered) == 0 {
		return Summary{Success: true}
	}
	if opts.Command == nil {
		opts.Command = defaultCommand
	}

	summary := Summary{Success: true}
	if opts.Output != nil {
		_, _ = fmt.Fprintf(opts.Output, "[verify] running %d %s gate(s) (parallel=%v)\n", len(filtered), phase, opts.Parallel)
	}

	if opts.Parallel {
		results := make([]Result, len(filtered))
		var wg sync.WaitGroup
		for i, gate := range filtered {
			wg.Add(1)
			go func(idx int, g Gate) {
				defer wg.Done()
				results[idx] = runGate(ctx, workDir, opts, g)
			}(i, gate)
		}
		wg.Wait()
		summary.Results = results
	} else {
		for _, gate := range filtered {
			result := runGate(ctx, workDir, opts, gate)
			summary.Results = append(summary.Results, result)
			if !result.Success {
				break
			}
		}
	}

	for _, result := range summary.Results {
		if !result.Success {
			summary.Success = false
			break
		}
	}
	return summary
}

func runGate(ctx context.Context, workDir string, opts RunOptions, gate Gate) Result {
	if opts.Output != nil {
		_, _ = fmt.Fprintf(opts.Output, "[verify] gate %q: %s\n", gate.Name, gate.Cmd)
	}
	start := time.Now()
	gateCtx := ctx
	cancel := func() {}
	if gate.Timeout > 0 {
		gateCtx, cancel = context.WithTimeout(ctx, gate.Timeout)
	}
	defer cancel()

	out, err := opts.Command(gateCtx, workDir, opts.Env, gate.Cmd)
	result := Result{
		Name:    gate.Name,
		Success: err == nil,
		Output:  strings.TrimSpace(string(out)),
		Elapsed: time.Since(start),
	}
	if err != nil {
		result.Error = err.Error()
		if result.Output != "" {
			result.Error = result.Error + ": " + truncateOutput(result.Output, 50)
		}
	}
	if opts.Output != nil {
		if result.Success {
			_, _ = fmt.Fprintf(opts.Output, "[verify] gate %q passed (%v)\n", gate.Name, result.Elapsed.Truncate(time.Millisecond))
		} else {
			_, _ = fmt.Fprintf(opts.Output, "[verify] gate %q failed (%v): %s\n", gate.Name, result.Elapsed.Truncate(time.Millisecond), result.Error)
		}
	}
	return result
}

func defaultCommand(ctx context.Context, workDir string, env []string, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // trusted repo/operator config
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

func truncateOutput(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return strings.TrimSpace(s)
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}
