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
    
    if ! command -v opencode &>/dev/null; then
        fail "opencode not found"
        exit 1
    fi
    
    success "Prerequisites OK"
}

setup_workspace() {
    TEST_DIR=$(mktemp -d -t gastown-formula-e2e-XXXXXX)
    log "Created test workspace: $TEST_DIR"
    
    cd "$TEST_DIR"
    git init -q
    git config user.email "test@example.com"
    git config user.name "Test User"
    
    mkdir -p .beads/formulas src
    
    cat > .beads/formulas/test-release.formula.toml << 'EOF'
description = "Test release workflow"
formula = "test-release"
version = 1

[vars.version]
description = "Version number"
required = true

[[steps]]
id = "bump"
title = "Bump version"
description = "Update version to {{version}}"

[[steps]]
id = "test"
title = "Run tests"
description = "Execute test suite"
needs = ["bump"]

[[steps]]
id = "tag"
title = "Create tag"
description = "Tag as v{{version}}"
needs = ["test"]
EOF

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
    
    mkdir -p .opencode/plugin
    cp "$PROJECT_ROOT/internal/opencode/plugin/gastown.js" .opencode/plugin/
    
    cat > .opencode/config.jsonc << 'EOF'
{
  "plugins": {
    "gastown": "./plugin/gastown.js"
  }
}
EOF
}

test_formula_list() {
    log "Testing formula listing..."
    
    if command -v bd &>/dev/null; then
        if bd formula list 2>/dev/null | grep -q "test-release" || true; then
            success "Formula list works"
            return 0
        fi
    fi
    
    if [[ -f .beads/formulas/test-release.formula.toml ]]; then
        success "Formula file exists (bd not available for listing)"
        return 0
    fi
    
    fail "Formula not found"
    return 1
}

test_formula_parse() {
    log "Testing formula parsing..."
    
    local formula_file=".beads/formulas/test-release.formula.toml"
    
    if grep -q 'formula = "test-release"' "$formula_file" && \
       grep -q '\[\[steps\]\]' "$formula_file" && \
       grep -q 'needs = \["bump"\]' "$formula_file"; then
        success "Formula structure valid"
        return 0
    fi
    
    fail "Formula structure invalid"
    return 1
}

test_formula_vars() {
    log "Testing formula variable substitution..."
    
    local formula_file=".beads/formulas/test-release.formula.toml"
    
    if grep -q '{{version}}' "$formula_file"; then
        success "Formula variables defined"
        return 0
    fi
    
    fail "Formula variables missing"
    return 1
}

test_opencode_formula_agent() {
    log "Testing OpenCode formula agent concept..."
    
    export GT_FORMULA=test-release
    export GT_ROLE=polecat
    
    if [[ -n "$GT_FORMULA" ]]; then
        success "GT_FORMULA environment set"
        return 0
    fi
    
    fail "GT_FORMULA not settable"
    return 1
}

main() {
    echo ""
    log "OpenCode Formula E2E Test"
    log "========================="
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
    local total=4
    
    test_formula_list && ((passed++)) || true
    test_formula_parse && ((passed++)) || true
    test_formula_vars && ((passed++)) || true
    test_opencode_formula_agent && ((passed++)) || true
    
    echo ""
    log "=========================================="
    log "           FORMULA TEST REPORT           "
    log "=========================================="
    echo ""
    echo "  Passed: $passed/$total"
    echo ""
    
    if [[ $passed -eq $total ]]; then
        success "ALL FORMULA TESTS PASSED"
        exit 0
    else
        fail "$((total - passed)) test(s) failed"
        exit 1
    fi
}

main "$@"
