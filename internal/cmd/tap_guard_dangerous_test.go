package cmd

import (
	"strings"
	"testing"
)

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"valid hook input", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /tmp/foo"}}`, "rm -rf /tmp/foo"},
		{"empty input", "", ""},
		{"invalid json", "not json", ""},
		{"no command field", `{"tool_name":"Write","tool_input":{"file_path":"/tmp/foo"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCommand([]byte(tt.input))
			if got != tt.want {
				t.Errorf("extractCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchesAllFragments(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		fragments []string
		want      bool
	}{
		{"git reset hard", "git reset --hard", []string{"git", "reset", "--hard"}, true},
		{"git reset soft", "git reset --soft", []string{"git", "reset", "--hard"}, false},
		{"drop table", "drop table users", []string{"drop", "table"}, true},
		{"drop database", "drop database mydb", []string{"drop", "database"}, true},
		{"truncate table", "truncate table logs", []string{"truncate", "table"}, true},
		{"git clean -f", "git clean -f", []string{"git", "clean", "-f"}, true},
		{"git clean -n", "git clean -n", []string{"git", "clean", "-f"}, false},
		{"no match", "echo hello", []string{"rm", "-rf"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAllFragments(tt.command, tt.fragments)
			if got != tt.want {
				t.Errorf("matchesAllFragments(%q, %v) = %v, want %v", tt.command, tt.fragments, got, tt.want)
			}
		})
	}
}

func TestMatchesDangerousRmRf(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Should block
		{"rm -rf /", "rm -rf /", true},
		{"rm -rf /*", "rm -rf /*", true},
		{"rm -rf / with sudo", "sudo rm -rf /", true},

		// Should allow (normal cleanup commands)
		{"rm -rf ./build/", "rm -rf ./build/", false},
		{"rm -rf node_modules/", "rm -rf node_modules/", false},
		{"rm -rf /tmp/test-output/", "rm -rf /tmp/test-output/", false},
		{"rm -rf relative dir", "rm -rf build", false},
		{"rm single file", "rm foo.txt", false},
		{"rm -r no force", "rm -r /", false},
		{"no rm at all", "echo hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesDangerousRmRf(tt.command) != ""
			if got != tt.blocked {
				t.Errorf("matchesDangerousRmRf(%q) blocked=%v, want %v", tt.command, got, tt.blocked)
			}
		})
	}
}

func TestMatchesDangerousGitPush(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Should block
		{"git push --force", "git push --force origin main", true},
		{"git push -f", "git push -f origin main", true},
		{"git push --force bare", "git push --force", true},

		// Should allow (safe variants)
		{"force-with-lease", "git push --force-with-lease origin main", false},
		{"force-if-includes", "git push --force-if-includes origin main", false},
		{"normal push", "git push origin main", false},
		{"no push", "git status", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesDangerousGitPush(tt.command) != ""
			if got != tt.blocked {
				t.Errorf("matchesDangerousGitPush(%q) blocked=%v, want %v", tt.command, got, tt.blocked)
			}
		})
	}
}

func TestMatchesSudo(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Should block
		{"sudo dnf install", "sudo dnf install -y postgresql-contrib", true},
		{"sudo rm", "sudo rm -rf /var/log/syslog", true},
		{"sudo bare", "sudo su", true},
		{"sudo in pipeline", "echo foo | sudo tee /etc/config", true},

		// Should allow
		{"no sudo", "echo hello", false},
		{"sudo in string", "echo 'do not use sudo'", false}, // contains "sudo" as substring of different token? Actually "sudo" IS a token here
		{"pseudocode", "cat pseudocode.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSudo(strings.ToLower(tt.command)) != ""
			if got != tt.blocked {
				t.Errorf("matchesSudo(%q) blocked=%v, want %v", tt.command, got, tt.blocked)
			}
		})
	}
}

func TestMatchesPackageInstall(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Should block
		{"apt install", "apt install -y curl", true},
		{"apt-get install", "apt-get install -y build-essential", true},
		{"dnf install", "dnf install -y postgresql-contrib", true},
		{"yum install", "yum install -y gcc", true},
		{"pacman -S", "pacman -S git", true},
		{"brew install", "brew install node", true},
		{"gem install", "gem install bundler", true},
		{"pip install --system", "pip install --system requests", true},
		{"pip3 install --system", "pip3 install --system flask", true},
		{"npm install -g", "npm install -g typescript", true},
		{"npm install --global", "npm install --global eslint", true},

		// Should allow
		{"pip install (venv ok)", "pip install requests", false},
		{"npm install (local ok)", "npm install express", false},
		{"npm install --save-dev", "npm install --save-dev jest", false},
		{"go install", "go install ./...", false},
		{"cargo install", "cargo install ripgrep", false},
		{"normal command", "ls -la", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPackageInstall(strings.ToLower(tt.command)) != ""
			if got != tt.blocked {
				t.Errorf("matchesPackageInstall(%q) blocked=%v, want %v", tt.command, got, tt.blocked)
			}
		})
	}
}

// TestDangerousGuard_Integration tests the full pattern set end-to-end.
func TestDangerousGuard_Integration(t *testing.T) {
	tests := []struct {
		name    string
		command string
		blocked bool
	}{
		// Blocked — privilege escalation
		{"sudo command", "sudo dnf install -y foo", true},
		{"sudo rm", "sudo rm -rf /var/cache", true},

		// Blocked — package installs
		{"apt install", "apt install -y curl", true},
		{"dnf install", "dnf install -y postgresql-contrib", true},
		{"brew install", "brew install node", true},
		{"npm install -g", "npm install -g typescript", true},
		{"pip install --system", "pip install --system requests", true},

		// Blocked — destructive operations
		{"rm -rf /", "rm -rf /", true},
		{"git push --force", "git push --force origin main", true},
		{"git reset --hard", "git reset --hard HEAD~1", true},
		{"git clean -f", "git clean -f", true},
		{"git clean -fd", "git clean -fd", true},
		{"drop table", "DROP TABLE users", true},

		// Allowed
		{"rm -rf ./build/", "rm -rf ./build/", false},
		{"rm -rf /tmp/cache/", "rm -rf /tmp/cache/", false},
		{"git push --force-with-lease", "git push --force-with-lease origin main", false},
		{"git push normal", "git push origin main", false},
		{"git reset soft", "git reset --soft HEAD~1", false},
		{"pip install (venv)", "pip install requests", false},
		{"npm install (local)", "npm install express", false},
		{"normal command", "ls -la", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lower := strings.ToLower(tt.command)
			blocked := false
			if matchesSudo(lower) != "" {
				blocked = true
			} else if matchesPackageInstall(lower) != "" {
				blocked = true
			} else if matchesDangerousRmRf(lower) != "" {
				blocked = true
			} else if matchesDangerousGitPush(lower) != "" {
				blocked = true
			} else {
				for _, p := range fragmentPatterns {
					if matchesAllFragments(lower, p.contains) {
						blocked = true
						break
					}
				}
			}
			if blocked != tt.blocked {
				t.Errorf("command %q: blocked=%v, want %v", tt.command, blocked, tt.blocked)
			}
		})
	}
}
