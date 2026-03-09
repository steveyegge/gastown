package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	copilotagent "github.com/steveyegge/gastown/internal/agent/copilot"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	copilotRunOnce       bool
	copilotPollInterval  time.Duration
	copilotTimeout       time.Duration
	copilotModel         string
	copilotCLIPath       string
	copilotCLIURL        string
	copilotLogLevel      string
	copilotLogFile       string
	copilotAllowAllTools bool
)

var copilotCmd = &cobra.Command{
	Use:   "copilot",
	Short: "Run Copilot SDK workers",
}

var copilotRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a Copilot worker loop",
	RunE:  runCopilotRun,
}

func init() {
	copilotRunCmd.Flags().BoolVar(&copilotRunOnce, "once", false, "Run one Copilot task and exit")
	copilotRunCmd.Flags().DurationVar(&copilotPollInterval, "poll-interval", 15*time.Second, "Poll interval for new mail")
	copilotRunCmd.Flags().DurationVar(&copilotTimeout, "timeout", 10*time.Minute, "Timeout for a single Copilot run")
	copilotRunCmd.Flags().StringVar(&copilotModel, "model", "", "Copilot model ID (defaults to Copilot CLI default)")
	copilotRunCmd.Flags().StringVar(&copilotCLIPath, "cli-path", "", "Copilot CLI binary path (defaults to copilot)")
	copilotRunCmd.Flags().StringVar(&copilotCLIURL, "cli-url", "", "Connect to an existing Copilot CLI server")
	copilotRunCmd.Flags().StringVar(&copilotLogLevel, "log-level", "info", "Copilot CLI log level")
	copilotRunCmd.Flags().StringVar(&copilotLogFile, "log-file", "", "Log file for Copilot tool activity")
	copilotRunCmd.Flags().BoolVar(&copilotAllowAllTools, "allow-all-tools", true, "Allow all Copilot tools")

	copilotCmd.AddCommand(copilotRunCmd)
	rootCmd.AddCommand(copilotCmd)
}

type copilotMailStatus struct {
	Address string `json:"address"`
	Unread  int    `json:"unread"`
	HasNew  bool   `json:"has_new"`
}

func runCopilotRun(cmd *cobra.Command, _ []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting work dir: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	roleInfo, err := GetRoleWithContext(workDir, townRoot)
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	systemPrompt, err := renderCopilotSystemPrompt(roleInfo, townRoot, workDir)
	if err != nil {
		return err
	}

	logWriter, closer, err := openCopilotLogWriter(workDir, roleInfo)
	if err != nil {
		return err
	}
	defer func() {
		if closer != nil {
			_ = closer.Close()
		}
	}()

	runner, err := copilotagent.NewRunner(copilotagent.Config{
		CLIPath:   copilotCLIPath,
		CLIURL:    copilotCLIURL,
		LogLevel:  copilotLogLevel,
		Model:     copilotModel,
		WorkDir:   workDir,
		Timeout:   copilotTimeout,
		LogWriter: logWriter,
		AllowAll:  copilotAllowAllTools,
	})
	if err != nil {
		return err
	}
	defer runner.Close()

	runOnce := func() error {
		prompt, err := buildCopilotPrompt(workDir, roleInfo)
		if err != nil {
			return err
		}
		_, err = runner.RunOnce(cmd.Context(), systemPrompt, prompt)
		return err
	}

	if copilotRunOnce {
		return runOnce()
	}

	for {
		status, err := getMailStatus(workDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "copilot: mail check failed: %v\n", err)
		} else if status.HasNew {
			if err := runOnce(); err != nil {
				fmt.Fprintf(os.Stderr, "copilot: run failed: %v\n", err)
			}
		}
		time.Sleep(copilotPollInterval)
	}
}

func renderCopilotSystemPrompt(roleInfo RoleInfo, townRoot, workDir string) (string, error) {
	townName, _ := workspace.GetTownName(townRoot)
	roleName := string(roleInfo.Role)

	defaultBranch := "main"
	issuePrefix := ""
	if roleInfo.Rig != "" {
		rigPath := filepath.Join(townRoot, roleInfo.Rig)
		if cfg, err := rig.LoadRigConfig(rigPath); err == nil && cfg.DefaultBranch != "" {
			defaultBranch = cfg.DefaultBranch
		}
		issuePrefix = beads.GetPrefixForRig(townRoot, roleInfo.Rig)
	}

	beadsDir := beads.ResolveBeadsDir(workDir)
	tmpl, err := templates.New()
	if err != nil {
		return "", fmt.Errorf("loading templates: %w", err)
	}

	prompt, err := tmpl.RenderRole(roleName, templates.RoleData{
		Role:          roleName,
		RigName:       roleInfo.Rig,
		TownRoot:      townRoot,
		TownName:      townName,
		WorkDir:       workDir,
		DefaultBranch: defaultBranch,
		Polecat:       roleInfo.Polecat,
		BeadsDir:      beadsDir,
		IssuePrefix:   issuePrefix,
		MayorSession:  session.MayorSessionName(),
		DeaconSession: session.DeaconSessionName(),
	})
	if err != nil {
		return fmt.Sprintf("You are a Gas Town %s agent. Follow the Gas Town workflow, use gt mail for tasks, and keep work scoped to this repo.", roleName), nil
	}

	return prompt, nil
}

func buildCopilotPrompt(workDir string, roleInfo RoleInfo) (string, error) {
	primeOutput, _ := runGtCommand(workDir, "prime", "--dry-run")
	mailInject, _ := runGtCommand(workDir, "mail", "check", "--inject")

	sections := []string{
		"# Gas Town Context",
		strings.TrimSpace(primeOutput),
		strings.TrimSpace(mailInject),
		"# Instructions",
		"Check your mail inbox for assignments. Use `gt mail inbox` and `gt mail read <id>` to see details. Follow the role instructions, make changes in this workspace, and report status with `gt handoff` or `gt done` as appropriate.",
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n")), nil
}

func getMailStatus(workDir string) (copilotMailStatus, error) {
	output, err := runGtCommand(workDir, "mail", "check", "--json")
	if err != nil {
		return copilotMailStatus{}, err
	}
	var status copilotMailStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return copilotMailStatus{}, fmt.Errorf("parsing mail status: %w", err)
	}
	return status, nil
}

func runGtCommand(workDir string, args ...string) (string, error) {
	cmdPath := os.Args[0]
	if !filepath.IsAbs(cmdPath) {
		if resolved, err := exec.LookPath(cmdPath); err == nil {
			cmdPath = resolved
		}
	}

	command := exec.Command(cmdPath, args...)
	command.Dir = workDir
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("gt %s failed: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func openCopilotLogWriter(workDir string, roleInfo RoleInfo) (io.Writer, io.Closer, error) {
	if copilotLogFile == "" {
		logsDir := filepath.Join(workDir, ".logs")
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("creating logs dir: %w", err)
		}
		roleSuffix := roleInfo.Rig
		if roleInfo.Polecat != "" {
			roleSuffix = roleInfo.Polecat
		}
		copilotLogFile = filepath.Join(logsDir, fmt.Sprintf("copilot-%s.log", roleSuffix))
	}

	file, err := os.OpenFile(copilotLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("opening log file: %w", err)
	}

	return file, file, nil
}
