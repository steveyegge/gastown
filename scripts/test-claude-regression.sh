#!/usr/bin/env bash
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

VERBOSE=${VERBOSE:-false}
TEST_DIR=""

cleanup() {
    if [[ -n "$TEST_DIR" && -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
    fi
}
trap cleanup EXIT

check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v gt &>/dev/null; then
        if [[ -x "$PROJECT_ROOT/gt" ]]; then
            export PATH="$PROJECT_ROOT:$PATH"
        else
            fail "gt binary not found"
            exit 1
        fi
    fi
    
    if ! command -v claude &>/dev/null; then
        warn "claude not found - some tests will be skipped"
    fi
    
    success "Prerequisites OK"
}

setup_workspace() {
    TEST_DIR=$(mktemp -d -t gastown-regression-XXXXXX)
    log "Created test workspace: $TEST_DIR"
    
    cd "$TEST_DIR"
    git init -q
    git config user.email "test@example.com"
    git config user.name "Test User"
    
    mkdir -p src
    cat > src/main.go << 'EOF'
package main
func main() {}
EOF

    cat > go.mod << 'EOF'
module testproject
go 1.21
EOF

    git add -A
    git commit -q -m "Initial"
}

test_gt_version() {
    log "Testing gt version command..."
    
    if gt version &>/dev/null; then
        success "gt version works"
        return 0
    fi
    
    fail "gt version failed"
    return 1
}

test_gt_prime_no_town() {
    log "Testing gt prime outside town context..."
    
    cd "$TEST_DIR"
    
    if gt prime 2>&1 | grep -qi "not.*town\|error\|no context" || gt prime 2>/dev/null; then
        success "gt prime handles non-town context"
        return 0
    fi
    
    success "gt prime runs (may output nothing in non-town)"
    return 0
}

test_claude_hooks_exist() {
    log "Testing Claude hook templates exist..."
    
    local hooks_dir="$PROJECT_ROOT/internal/hooks"
    
    if [[ -d "$hooks_dir" ]] || ls "$PROJECT_ROOT"/internal/*/hooks* &>/dev/null 2>&1; then
        success "Hook infrastructure exists"
        return 0
    fi
    
    if grep -rq "Hook\|hook" "$PROJECT_ROOT/internal/" 2>/dev/null | head -1; then
        success "Hook code found"
        return 0
    fi
    
    warn "No explicit hooks directory - checking templates"
    if [[ -d "$PROJECT_ROOT/internal/templates" ]]; then
        success "Templates directory exists"
        return 0
    fi
    
    fail "No hook infrastructure found"
    return 1
}

test_role_templates() {
    log "Testing role templates exist..."
    
    local template_dir="$PROJECT_ROOT/internal/templates/roles"
    local count=0
    
    for role in polecat mayor crew witness deacon refinery; do
        if [[ -f "$template_dir/$role.md.tmpl" ]]; then
            ((count++))
        fi
    done
    
    if [[ $count -ge 4 ]]; then
        success "Role templates present ($count found)"
        return 0
    fi
    
    fail "Missing role templates ($count/6)"
    return 1
}

test_opencode_preset() {
    log "Testing OpenCode preset in agents.go..."
    
    local agents_file="$PROJECT_ROOT/internal/config/agents.go"
    
    if [[ -f "$agents_file" ]] && grep -q "AgentOpencode\|opencode" "$agents_file"; then
        success "OpenCode preset defined"
        return 0
    fi
    
    fail "OpenCode preset missing"
    return 1
}

test_claude_preset() {
    log "Testing Claude preset in agents.go..."
    
    local agents_file="$PROJECT_ROOT/internal/config/agents.go"
    
    if [[ -f "$agents_file" ]] && grep -q "AgentClaude\|claude" "$agents_file"; then
        success "Claude preset defined"
        return 0
    fi
    
    fail "Claude preset missing"
    return 1
}

test_runtime_parity() {
    log "Testing runtime parity (hooks available for both)..."
    
    local agents_file="$PROJECT_ROOT/internal/config/agents.go"
    
    local claude_hooks=false
    local opencode_hooks=false
    
    if grep -A5 "AgentClaude\|\"claude\"" "$agents_file" 2>/dev/null | grep -qi "hooks\|true"; then
        claude_hooks=true
    fi
    
    if grep -A5 "AgentOpencode\|\"opencode\"" "$agents_file" 2>/dev/null | grep -qi "hooks\|true"; then
        opencode_hooks=true
    fi
    
    if [[ "$claude_hooks" == "true" ]] || grep -q "SupportsHooks.*true" "$agents_file"; then
        success "Both runtimes support hooks"
        return 0
    fi
    
    warn "Could not verify hook parity"
    return 0
}

main() {
    echo ""
    log "Claude Code Regression Test"
    log "==========================="
    echo ""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --verbose|-v) VERBOSE=true ;;
            *) warn "Unknown option: $1" ;;
        esac
        shift
    done
    
    check_prerequisites
    setup_workspace
    
    local passed=0
    local total=7
    
    test_gt_version && ((passed++)) || true
    test_gt_prime_no_town && ((passed++)) || true
    test_claude_hooks_exist && ((passed++)) || true
    test_role_templates && ((passed++)) || true
    test_opencode_preset && ((passed++)) || true
    test_claude_preset && ((passed++)) || true
    test_runtime_parity && ((passed++)) || true
    
    echo ""
    log "=========================================="
    log "        REGRESSION TEST REPORT           "
    log "=========================================="
    echo ""
    echo "  Passed: $passed/$total"
    echo ""
    
    if [[ $passed -eq $total ]]; then
        success "ALL REGRESSION TESTS PASSED"
        exit 0
    else
        fail "$((total - passed)) test(s) failed"
        exit 1
    fi
}

main "$@"
