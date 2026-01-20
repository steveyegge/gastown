#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[TEST]${NC} $*"; }
success() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

RUNTIME=${1:-""}
TIMEOUT=${TIMEOUT:-120}
TEST_DIR=""

cleanup() {
    if [[ -n "$TEST_DIR" && -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
    fi
}
trap cleanup EXIT

usage() {
    echo "Usage: $0 <runtime> [--tier N]"
    echo ""
    echo "Runtimes: claude, opencode"
    echo "Tiers:"
    echo "  1 - Simple task: create a file (30s)"
    echo "  2 - Bug fix task (2min)"
    echo "  3 - Feature addition (4min)"
    echo ""
    echo "Environment:"
    echo "  ANTHROPIC_API_KEY - Required for Claude"
    echo "  OPENAI_API_KEY    - May be required for OpenCode"
    echo ""
    echo "Example:"
    echo "  $0 claude --tier 1"
    echo "  $0 opencode --tier 2"
    exit 1
}

check_runtime() {
    case "$RUNTIME" in
        claude)
            if ! command -v claude &>/dev/null; then
                fail "claude CLI not found"
                exit 1
            fi
            if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
                fail "ANTHROPIC_API_KEY not set"
                exit 1
            fi
            ;;
        opencode)
            if ! command -v opencode &>/dev/null; then
                fail "opencode CLI not found"
                exit 1
            fi
            ;;
        *)
            usage
            ;;
    esac
    success "$RUNTIME ready"
}

run_agent() {
    local prompt="$1"
    local workdir="$2"
    
    log "Running $RUNTIME with prompt..."
    
    case "$RUNTIME" in
        claude)
            timeout "$TIMEOUT" claude -p "$prompt" --dangerously-skip-permissions 2>&1 || true
            ;;
        opencode)
            timeout "$TIMEOUT" opencode run "$prompt" 2>&1 || true
            ;;
    esac
}

tier1_create_file() {
    log "=== TIER 1: Create File Test ==="
    
    TEST_DIR=$(mktemp -d -t gastown-e2e-tier1-XXXXXX)
    cd "$TEST_DIR"
    
    cat > go.mod << 'EOF'
module testproject
go 1.21
EOF
    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"
    git add go.mod
    git commit -q -m "init"
    
    local prompt="Create a file called hello.go with a main function that prints Hello World. Just create the file, nothing else."
    
    run_agent "$prompt" "$TEST_DIR"
    
    if [[ -f "hello.go" ]]; then
        success "hello.go created"
        
        if go build -o hello . 2>/dev/null; then
            success "hello.go compiles"
            
            local output
            output=$(./hello 2>&1 || true)
            if echo "$output" | grep -qi "hello"; then
                success "Output contains 'hello': $output"
                return 0
            else
                fail "Output doesn't contain 'hello': $output"
                return 1
            fi
        else
            fail "hello.go doesn't compile"
            cat hello.go
            return 1
        fi
    else
        fail "hello.go not created"
        ls -la
        return 1
    fi
}

tier2_fix_bug() {
    log "=== TIER 2: Fix Bug Test ==="
    
    TEST_DIR=$(mktemp -d -t gastown-e2e-tier2-XXXXXX)
    cd "$TEST_DIR"
    
    cat > go.mod << 'EOF'
module testproject
go 1.21
EOF

    cat > math.go << 'EOF'
package main

func subtract(a, b int) int {
    return a + b
}
EOF

    cat > math_test.go << 'EOF'
package main

import "testing"

func TestSubtract(t *testing.T) {
    got := subtract(5, 3)
    want := 2
    if got != want {
        t.Errorf("subtract(5, 3) = %d, want %d", got, want)
    }
}
EOF

    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"
    git add .
    git commit -q -m "init"
    
    if go test ./... 2>/dev/null; then
        fail "Tests should fail before fix"
        return 1
    fi
    success "Tests fail before fix (expected)"
    
    local prompt="The subtract function in math.go has a bug - it adds instead of subtracts. Fix it so the tests pass."
    
    run_agent "$prompt" "$TEST_DIR"
    
    if go test ./... 2>/dev/null; then
        success "Tests pass after fix"
        return 0
    else
        fail "Tests still fail after fix"
        cat math.go
        return 1
    fi
}

tier3_add_feature() {
    log "=== TIER 3: Add Feature Test ==="
    
    TEST_DIR=$(mktemp -d -t gastown-e2e-tier3-XXXXXX)
    cd "$TEST_DIR"
    
    cat > go.mod << 'EOF'
module testproject
go 1.21
EOF

    cat > calculator.go << 'EOF'
package main

type Calculator struct {
    memory float64
}

func NewCalculator() *Calculator {
    return &Calculator{}
}

func (c *Calculator) Add(a, b float64) float64 {
    result := a + b
    c.memory = result
    return result
}

func (c *Calculator) Memory() float64 {
    return c.memory
}
EOF

    cat > calculator_test.go << 'EOF'
package main

import "testing"

func TestAdd(t *testing.T) {
    c := NewCalculator()
    got := c.Add(2, 3)
    if got != 5 {
        t.Errorf("Add(2, 3) = %f, want 5", got)
    }
}
EOF

    git init -q
    git config user.email "test@test.com"
    git config user.name "Test"
    git add .
    git commit -q -m "init"
    
    local prompt="Add a Multiply method to the Calculator that multiplies two float64 numbers and stores the result in memory. Also add a test for the Multiply method."
    
    run_agent "$prompt" "$TEST_DIR"
    
    if ! grep -q "Multiply" calculator.go; then
        fail "Multiply method not added"
        cat calculator.go
        return 1
    fi
    success "Multiply method added"
    
    if ! grep -q "Multiply" calculator_test.go; then
        fail "Multiply test not added"
        cat calculator_test.go
        return 1
    fi
    success "Multiply test added"
    
    if go test ./... 2>/dev/null; then
        success "All tests pass"
        return 0
    else
        fail "Tests fail"
        go test -v ./... 2>&1 || true
        return 1
    fi
}

main() {
    echo ""
    log "Real Agent E2E Test"
    log "==================="
    echo ""
    
    if [[ -z "$RUNTIME" ]]; then
        usage
    fi
    
    local tier="1"
    shift || true
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --tier) tier="$2"; shift 2 ;;
            *) usage ;;
        esac
    done
    
    check_runtime
    
    local result=0
    case "$tier" in
        1) tier1_create_file || result=1 ;;
        2) tier2_fix_bug || result=1 ;;
        3) tier3_add_feature || result=1 ;;
        all)
            tier1_create_file || result=1
            tier2_fix_bug || result=1
            tier3_add_feature || result=1
            ;;
        *) usage ;;
    esac
    
    echo ""
    if [[ $result -eq 0 ]]; then
        success "E2E test passed for $RUNTIME tier $tier"
    else
        fail "E2E test failed for $RUNTIME tier $tier"
    fi
    
    exit $result
}

main "$@"
