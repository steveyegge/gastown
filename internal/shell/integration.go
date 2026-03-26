// ABOUTME: Shell integration installation and removal for Gas Town.
// ABOUTME: Manages the shell hook in RC files with safe block markers.

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/steveyegge/gastown/internal/state"
)

const (
	markerStart = "# --- Gas Town Integration (managed by gt) ---"
	markerEnd   = "# --- End Gas Town ---"
)

func hookSourceLine() string {
	return fmt.Sprintf(`[[ -f "%s/shell-hook.sh" ]] && source "%s/shell-hook.sh"`,
		state.ConfigDir(), state.ConfigDir())
}

// pwshProfileSourceLine returns the PowerShell profile dot-source line.
func pwshProfileSourceLine() string {
	return fmt.Sprintf(`. "%s"`,
		filepath.Join(state.ConfigDir(), "shell-hook.ps1"))
}

func Install() error {
	shell := DetectShell()
	if shell == "powershell" {
		return installPowerShell()
	}
	rcPath := RCFilePath(shell)

	if err := writeHookScript(); err != nil {
		return fmt.Errorf("writing hook script: %w", err)
	}

	if err := addToRCFile(rcPath); err != nil {
		return fmt.Errorf("updating %s: %w", rcPath, err)
	}

	return state.SetShellIntegration(shell)
}

// installPowerShell installs the Gas Town hook into the PowerShell profile.
func installPowerShell() error {
	if err := writePowerShellHookScript(); err != nil {
		return fmt.Errorf("writing PowerShell hook script: %w", err)
	}

	profilePath := RCFilePath("powershell")
	if err := addToRCFile(profilePath); err != nil {
		return fmt.Errorf("updating %s: %w", profilePath, err)
	}

	return state.SetShellIntegration("powershell")
}

func Remove() error {
	shell := DetectShell()
	rcPath := RCFilePath(shell)

	if err := removeFromRCFile(rcPath); err != nil {
		return fmt.Errorf("updating %s: %w", rcPath, err)
	}

	// Remove the appropriate hook script.
	hookFile := "shell-hook.sh"
	if shell == "powershell" {
		hookFile = "shell-hook.ps1"
	}
	hookPath := filepath.Join(state.ConfigDir(), hookFile)
	if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing hook script: %w", err)
	}

	return nil
}

func DetectShell() string {
	// On Windows, prefer PowerShell unless running inside WSL/Git Bash.
	if runtime.GOOS == "windows" {
		// Check if we're in a POSIX-like environment (WSL, Git Bash, MSYS2).
		if shell := os.Getenv("SHELL"); shell != "" {
			if strings.HasSuffix(shell, "bash") {
				return "bash"
			}
			if strings.HasSuffix(shell, "zsh") {
				return "zsh"
			}
		}
		return "powershell"
	}
	shell := os.Getenv("SHELL")
	if strings.HasSuffix(shell, "zsh") {
		return "zsh"
	}
	if strings.HasSuffix(shell, "bash") {
		return "bash"
	}
	return "zsh"
}

func RCFilePath(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "powershell":
		// PowerShell profile: $HOME\Documents\PowerShell\Microsoft.PowerShell_profile.ps1
		// Works for both PowerShell 7+ (pwsh) and Windows PowerShell 5.1.
		docs := filepath.Join(home, "Documents", "PowerShell")
		return filepath.Join(docs, "Microsoft.PowerShell_profile.ps1")
	default:
		return filepath.Join(home, ".zshrc")
	}
}

func writeHookScript() error {
	dir := state.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	hookPath := filepath.Join(dir, "shell-hook.sh")
	return os.WriteFile(hookPath, []byte(shellHookScript), 0644)
}

func addToRCFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)

	if strings.Contains(content, markerStart) {
		return updateRCFile(path, content)
	}

	// Use the appropriate source line based on file type.
	sourceLine := hookSourceLine()
	if strings.HasSuffix(path, ".ps1") {
		sourceLine = pwshProfileSourceLine()
	}

	block := fmt.Sprintf("\n%s\n%s\n%s\n", markerStart, sourceLine, markerEnd)

	if len(data) > 0 {
		backupPath := path + ".gastown-backup"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("writing backup: %w", err)
		}
	}

	return os.WriteFile(path, []byte(content+block), 0644)
}

func removeFromRCFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	startIdx := strings.Index(content, markerStart)
	if startIdx == -1 {
		return nil
	}

	endIdx := strings.Index(content[startIdx:], markerEnd)
	if endIdx == -1 {
		return nil
	}
	endIdx += startIdx + len(markerEnd)

	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}

	if startIdx > 0 && content[startIdx-1] == '\n' {
		startIdx--
	}

	newContent := content[:startIdx] + content[endIdx:]
	return os.WriteFile(path, []byte(newContent), 0644)
}

func updateRCFile(path, content string) error {
	startIdx := strings.Index(content, markerStart)
	endIdx := strings.Index(content[startIdx:], markerEnd)
	if endIdx == -1 {
		return fmt.Errorf("malformed Gas Town block in %s", path)
	}
	endIdx += startIdx + len(markerEnd)

	sourceLine := hookSourceLine()
	if strings.HasSuffix(path, ".ps1") {
		sourceLine = pwshProfileSourceLine()
	}

	block := fmt.Sprintf("%s\n%s\n%s", markerStart, sourceLine, markerEnd)
	newContent := content[:startIdx] + block + content[endIdx:]

	return os.WriteFile(path, []byte(newContent), 0644)
}

var shellHookScript = `#!/bin/bash
# Gas Town Shell Integration
# Installed by: gt install --shell
# Location: ~/.config/gastown/shell-hook.sh

_gastown_enabled() {
    [[ -n "$GASTOWN_DISABLED" ]] && return 1
    [[ -n "$GASTOWN_ENABLED" ]] && return 0
    local state_file="$HOME/.local/state/gastown/state.json"
    [[ -f "$state_file" ]] && grep -q '"enabled":\s*true' "$state_file" 2>/dev/null
}

_gastown_ignored() {
    local dir="$PWD"
    while [[ "$dir" != "/" ]]; do
        [[ -f "$dir/.gastown-ignore" ]] && return 0
        dir="$(dirname "$dir")"
    done
    return 1
}

_gastown_already_asked() {
    local repo_root="$1"
    local asked_file="$HOME/.cache/gastown/asked-repos"
    [[ -f "$asked_file" ]] && grep -qF "$repo_root" "$asked_file" 2>/dev/null
}

_gastown_mark_asked() {
    local repo_root="$1"
    local asked_file="$HOME/.cache/gastown/asked-repos"
    mkdir -p "$(dirname "$asked_file")"
    echo "$repo_root" >> "$asked_file"
}

_gastown_offer_add() {
    local repo_root="$1"

    [[ "${GASTOWN_DISABLE_OFFER_ADD:-}" == "1" ]] && return 0
    _gastown_already_asked "$repo_root" && return 0
    
    [[ -t 0 ]] || return 0
    
    local repo_name
    repo_name=$(basename "$repo_root")
    
    echo ""
    echo -n "Add '$repo_name' to Gas Town? [y/N/never] "
    read -r response </dev/tty
    
    _gastown_mark_asked "$repo_root"
    
    case "$response" in
        y|Y|yes)
            echo "Adding to Gas Town..."
            local output
            output=$(gt rig quick-add "$repo_root" --yes 2>&1)
            local exit_code=$?
            echo "$output"
            
            if [[ $exit_code -eq 0 ]]; then
                local crew_path
                crew_path=$(echo "$output" | grep "^GT_CREW_PATH=" | cut -d= -f2)
                if [[ -n "$crew_path" && -d "$crew_path" ]]; then
                    echo ""
                    echo "Switching to crew workspace..."
                    cd "$crew_path" || true
                    # Re-run hook to set GT_TOWN_ROOT and GT_RIG
                    _gastown_hook
                fi
            fi
            ;;
        never)
            touch "$repo_root/.gastown-ignore"
            echo "Created .gastown-ignore - won't ask again for this repo."
            ;;
        *)
            echo "Skipped. Run 'gt rig quick-add' later to add manually."
            ;;
    esac
}

_gastown_hook() {
    local previous_exit_status=$?

    _gastown_enabled || {
        unset GT_TOWN_ROOT GT_RIG
        return $previous_exit_status
    }

    _gastown_ignored && {
        unset GT_TOWN_ROOT GT_RIG
        return $previous_exit_status
    }

    if ! git rev-parse --git-dir &>/dev/null; then
        unset GT_TOWN_ROOT GT_RIG
        return $previous_exit_status
    fi

    local repo_root
    repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
        unset GT_TOWN_ROOT GT_RIG
        return $previous_exit_status
    }

    local cache_file="$HOME/.cache/gastown/rigs.cache"
    if [[ -f "$cache_file" ]]; then
        local cached
        cached=$(grep "^${repo_root}:" "$cache_file" 2>/dev/null)
        if [[ -n "$cached" ]]; then
            eval "${cached#*:}"
            return $previous_exit_status
        fi
    fi

    if command -v gt &>/dev/null; then
        local detect_output
        detect_output=$(gt rig detect "$repo_root" 2>/dev/null)
        eval "$detect_output"
        
        if [[ -n "$GT_TOWN_ROOT" ]]; then
            (gt rig detect --cache "$repo_root" &>/dev/null &)
        elif [[ -n "$_GASTOWN_OFFER_ADD" ]]; then
            _gastown_offer_add "$repo_root"
            unset _GASTOWN_OFFER_ADD
        fi
    fi

    return $previous_exit_status
}

_gastown_chpwd_hook() {
    _GASTOWN_OFFER_ADD=1
    _gastown_hook
}

case "${SHELL##*/}" in
    zsh)
        autoload -Uz add-zsh-hook
        add-zsh-hook chpwd _gastown_chpwd_hook
        add-zsh-hook precmd _gastown_hook
        ;;
    bash)
        if [[ ";${PROMPT_COMMAND[*]:-};" != *";_gastown_hook;"* ]]; then
            PROMPT_COMMAND="_gastown_chpwd_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
        fi
        ;;
esac

_gastown_hook
`

// writePowerShellHookScript writes the PowerShell equivalent of shell-hook.sh.
func writePowerShellHookScript() error {
	dir := state.ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	hookPath := filepath.Join(dir, "shell-hook.ps1")
	return os.WriteFile(hookPath, []byte(pwshHookScript), 0644)
}

var pwshHookScript = `# Gas Town Shell Integration (PowerShell)
# Installed by: gt install --shell
# Location: ~/.config/gastown/shell-hook.ps1

function _gastown_enabled {
    if ($env:GASTOWN_DISABLED) { return $false }
    if ($env:GASTOWN_ENABLED) { return $true }
    $stateFile = Join-Path $HOME ".local" "state" "gastown" "state.json"
    if (Test-Path $stateFile) {
        $content = Get-Content $stateFile -Raw -ErrorAction SilentlyContinue
        return $content -match '"enabled"\s*:\s*true'
    }
    return $false
}

function _gastown_ignored {
    $dirPath = (Get-Location).Path
    while ($dirPath -ne [System.IO.Path]::GetPathRoot($dirPath)) {
        if (Test-Path (Join-Path $dirPath ".gastown-ignore")) { return $true }
        $dirPath = Split-Path $dirPath -Parent
        if (-not $dirPath) { break }
    }
    return $false
}

function _gastown_hook {
    if (-not (_gastown_enabled)) {
        Remove-Item Env:\GT_TOWN_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:\GT_RIG -ErrorAction SilentlyContinue
        return
    }

    if (_gastown_ignored) {
        Remove-Item Env:\GT_TOWN_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:\GT_RIG -ErrorAction SilentlyContinue
        return
    }

    $gitDir = git rev-parse --git-dir 2>$null
    if (-not $gitDir) {
        Remove-Item Env:\GT_TOWN_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:\GT_RIG -ErrorAction SilentlyContinue
        return
    }

    $repoRoot = git rev-parse --show-toplevel 2>$null
    if (-not $repoRoot) {
        Remove-Item Env:\GT_TOWN_ROOT -ErrorAction SilentlyContinue
        Remove-Item Env:\GT_RIG -ErrorAction SilentlyContinue
        return
    }

    $cacheFile = Join-Path $HOME ".cache" "gastown" "rigs.cache"
    if (Test-Path $cacheFile) {
        $cached = Select-String -Path $cacheFile -Pattern "^$([regex]::Escape($repoRoot)):" -ErrorAction SilentlyContinue
        if ($cached) {
            $vars = $cached.Line.Substring($cached.Line.IndexOf(':') + 1)
            # Parse KEY=VALUE pairs
            foreach ($pair in ($vars -split ';')) {
                if ($pair -match '^(\w+)=(.*)$') {
                    Set-Item "Env:\$($Matches[1])" $Matches[2]
                }
            }
            return
        }
    }

    if (Get-Command gt -ErrorAction SilentlyContinue) {
        $detectOutput = gt rig detect $repoRoot 2>$null
        if ($detectOutput) {
            foreach ($line in $detectOutput) {
                if ($line -match '^export\s+(\w+)="?([^"]*)"?$') {
                    Set-Item "Env:\$($Matches[1])" $Matches[2]
                } elseif ($line -match '^(\w+)=(.*)$') {
                    Set-Item "Env:\$($Matches[1])" $Matches[2]
                }
            }
        }
    }
}

# Run hook on prompt to detect Gas Town workspaces.
if (-not ($function:prompt -and ($function:prompt.ToString() -match '_gastown_hook'))) {
    $function:_gastown_original_prompt = $function:prompt
    function prompt {
        _gastown_hook
        _gastown_original_prompt
    }
}

_gastown_hook
`
