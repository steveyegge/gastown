#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== OpenCode TUI E2E Test ==="
echo "Testing TUI-based prompt injection approach"
echo ""

TEST_DIR=$(mktemp -d)
echo "Test Dir: $TEST_DIR"
cd "$TEST_DIR"

git init --initial-branch=main
git config user.email "test@test.com"
git config user.name "Test"

cat > math.go << 'EOF'
package main

func add(a, b int) int {
    return a + b
}

func subtract(a, b int) int {
    return a + b  // BUG: should be a - b
}
EOF

cat > math_test.go << 'EOF'
package main

import "testing"

func TestAdd(t *testing.T) {
    if add(2, 3) != 5 {
        t.Error("add(2, 3) should be 5")
    }
}

func TestSubtract(t *testing.T) {
    if subtract(5, 3) != 2 {
        t.Error("subtract(5, 3) should be 2")
    }
}
EOF

echo "module testmath" > go.mod
git add .
git commit -m "init with buggy subtract"

echo ""
echo "=== Starting OpenCode TUI with prompt file ==="

export XDG_CONFIG_HOME="$TEST_DIR/.config"
mkdir -p "$XDG_CONFIG_HOME/opencode"

cat > "$XDG_CONFIG_HOME/gastown_prompt.txt" << 'EOF'
There is a bug in math.go - the subtract function returns wrong results. 
Run the tests to see the failure, then fix the bug and verify tests pass.
EOF

export GT_BINARY_PATH="$PROJECT_ROOT/gt"
export GT_ROLE="polecat"

GASTOWN_PLUGIN="$PROJECT_ROOT/internal/opencode/plugin/gastown.js"
if [ ! -f "$GASTOWN_PLUGIN" ]; then
    echo "ERROR: gastown.js not found at $GASTOWN_PLUGIN"
    exit 1
fi

cat > "$TEST_DIR/.opencode.jsonc" << EOF
{
  "plugin": [
    "file://$GASTOWN_PLUGIN"
  ]
}
EOF

export OPENCODE_PLUGIN_DIR="$XDG_CONFIG_HOME/opencode/plugins"
mkdir -p "$OPENCODE_PLUGIN_DIR"

echo "Prompt: $(cat $XDG_CONFIG_HOME/gastown_prompt.txt | head -1)..."
echo "Plugin: $GASTOWN_PLUGIN"
echo "GT Binary: $GT_BINARY_PATH"
echo "Project config: $TEST_DIR/.opencode.jsonc"
echo ""

SESSION_NAME="oc-test-$$"
TMUX_TMPDIR=$(mktemp -d)
export TMUX_TMPDIR

cleanup() {
    echo ""
    echo "=== Cleanup ==="
    TMUX_TMPDIR="$TMUX_TMPDIR" tmux kill-session -t "$SESSION_NAME" 2>/dev/null || true
    rm -rf "$TEST_DIR" "$TMUX_TMPDIR"
}
trap cleanup EXIT

TMUX_TMPDIR="$TMUX_TMPDIR" tmux new-session -d -s "$SESSION_NAME" -x 200 -y 50

TMUX_TMPDIR="$TMUX_TMPDIR" tmux send-keys -t "$SESSION_NAME" "cd '$TEST_DIR' && opencode" Enter

echo "Waiting for opencode to start..."
sleep 5

echo ""
echo "=== Capturing TUI output ==="
for i in {1..60}; do
    OUTPUT=$(TMUX_TMPDIR="$TMUX_TMPDIR" tmux capture-pane -t "$SESSION_NAME" -p 2>/dev/null || echo "")
    
    if echo "$OUTPUT" | grep -q "GASTOWN_READY"; then
        echo "OpenCode ready (detected GASTOWN_READY)"
        break
    fi
    
    if echo "$OUTPUT" | grep -q "fixed\|Fixed\|modified\|PASS"; then
        echo "Work appears complete"
        break
    fi
    
    echo "[$i/60] Waiting... ($(echo "$OUTPUT" | tail -1 | head -c 80))"
    sleep 2
done

echo ""
echo "=== Final TUI State ==="
TMUX_TMPDIR="$TMUX_TMPDIR" tmux capture-pane -t "$SESSION_NAME" -p | tail -30

echo ""
echo "=== Checking math.go ==="
if grep -q "a - b" "$TEST_DIR/math.go"; then
    echo "SUCCESS: Bug was fixed (contains 'a - b')"
else
    echo "FAILURE: Bug not fixed"
    cat "$TEST_DIR/math.go"
fi

echo ""
echo "=== Running tests ==="
cd "$TEST_DIR"
go test -v ./... 2>&1 || echo "Tests completed"
