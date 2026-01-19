#!/bin/bash
# validate-opencode-docs.sh - Validate OpenCode documentation
#
# This script checks for common documentation issues and flags
# areas that may need review. See CONTRIBUTING.md for standards.
#
# Usage:
#   ./scripts/validate-opencode-docs.sh           # Full validation
#   ./scripts/validate-opencode-docs.sh --links   # Broken links only
#   ./scripts/validate-opencode-docs.sh --stale   # Stale docs only
#   ./scripts/validate-opencode-docs.sh --quick   # Fast checks for pre-commit

set -e

DOCS_DIR="docs/opencode"
CODE_DIR="internal/opencode"
CONFIG_FILE="internal/config/agents.go"
CONTRIBUTING="$DOCS_DIR/CONTRIBUTING.md"
HISTORY="$DOCS_DIR/HISTORY.md"

# Colors
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
ERRORS=0
WARNINGS=0

error() {
    echo -e "${RED}ERROR:${NC} $1"
    ((ERRORS++))
}

warn() {
    echo -e "${YELLOW}WARN:${NC} $1"
    ((WARNINGS++))
}

info() {
    echo -e "${BLUE}INFO:${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

# Parse arguments
CHECK_LINKS=true
CHECK_STALE=true
CHECK_HISTORY=true
CHECK_README=true
QUICK=false

for arg in "$@"; do
    case $arg in
        --links)
            CHECK_LINKS=true
            CHECK_STALE=false
            CHECK_HISTORY=false
            CHECK_README=false
            ;;
        --stale)
            CHECK_LINKS=false
            CHECK_STALE=true
            CHECK_HISTORY=false
            CHECK_README=false
            ;;
        --history)
            CHECK_LINKS=false
            CHECK_STALE=false
            CHECK_HISTORY=true
            CHECK_README=false
            ;;
        --quick)
            QUICK=true
            CHECK_STALE=false
            ;;
    esac
done

echo "═══════════════════════════════════════════════════════════"
echo " OpenCode Documentation Validation"
echo " Reference: $CONTRIBUTING"
echo "═══════════════════════════════════════════════════════════"
echo ""

# Check CONTRIBUTING.md exists
if [[ ! -f "$CONTRIBUTING" ]]; then
    error "CONTRIBUTING.md not found at $CONTRIBUTING"
fi

# ─────────────────────────────────────────────────────────────────
# 1. Check for broken links
# ─────────────────────────────────────────────────────────────────
if [[ "$CHECK_LINKS" == true ]]; then
    echo "Checking internal links..."
    
    BROKEN_LINKS=0
    while IFS= read -r file; do
        dir=$(dirname "$file")
        
        # Extract markdown links [text](path)
        # Skip external URLs (http://, https://)
        grep -oE '\[.+\]\([^)]+\)' "$file" 2>/dev/null | while read -r link; do
            path=$(echo "$link" | sed -E 's/.*\(([^)#]+).*/\1/')
            
            # Skip external links and anchors
            if [[ "$path" =~ ^https?:// ]] || [[ "$path" =~ ^#  ]] || [[ -z "$path" ]]; then
                continue
            fi
            
            # Resolve relative path
            if [[ "$path" =~ ^\.\./ ]] || [[ "$path" =~ ^\./ ]]; then
                target="$dir/$path"
            else
                target="$dir/$path"
            fi
            
            # Normalize path and check existence
            target=$(echo "$target" | sed 's|/\./|/|g')
            if [[ ! -f "$target" ]] && [[ ! -d "$target" ]]; then
                error "Broken link in $file: $path"
                ((BROKEN_LINKS++))
            fi
        done
    done < <(find "$DOCS_DIR" -name "*.md" -type f)
    
    if [[ $BROKEN_LINKS -eq 0 ]]; then
        success "All internal links valid"
    fi
    echo ""
fi

# ─────────────────────────────────────────────────────────────────
# 2. Check for stale documentation
# ─────────────────────────────────────────────────────────────────
if [[ "$CHECK_STALE" == true ]] && [[ -d "$CODE_DIR" ]]; then
    echo "Checking for stale documentation..."
    
    # Get most recent code modification
    CODE_MTIME=$(find "$CODE_DIR" -name "*.go" -o -name "*.js" -type f -exec stat -f %m {} \; 2>/dev/null | sort -rn | head -1)
    CONFIG_MTIME=$(stat -f %m "$CONFIG_FILE" 2>/dev/null || echo "0")
    
    # Key docs to check
    DOCS_TO_CHECK=(
        "$DOCS_DIR/reference/integration-guide.md"
        "$DOCS_DIR/reference/events.md"
        "$DOCS_DIR/reference/configuration.md"
        "$DOCS_DIR/reference/plugin-implementation.md"
    )
    
    STALE_COUNT=0
    for doc in "${DOCS_TO_CHECK[@]}"; do
        if [[ -f "$doc" ]]; then
            DOC_MTIME=$(stat -f %m "$doc" 2>/dev/null || echo "0")
            
            if [[ "$CODE_MTIME" -gt "$DOC_MTIME" ]] || [[ "$CONFIG_MTIME" -gt "$DOC_MTIME" ]]; then
                warn "Potentially stale: $doc (code modified more recently)"
                ((STALE_COUNT++))
            fi
        fi
    done
    
    if [[ $STALE_COUNT -eq 0 ]]; then
        success "All key docs up to date"
    else
        info "Review these docs for accuracy after code changes"
    fi
    echo ""
fi

# ─────────────────────────────────────────────────────────────────
# 3. Check HISTORY.md - Git-based change detection
# ─────────────────────────────────────────────────────────────────
if [[ "$CHECK_HISTORY" == true ]]; then
    echo "Checking HISTORY.md for recent changes..."
    
    if [[ -f "$HISTORY" ]]; then
        # Find the last commit that modified HISTORY.md
        LAST_HISTORY_COMMIT=$(git log -1 --format="%H" -- "$HISTORY" 2>/dev/null || echo "")
        
        if [[ -n "$LAST_HISTORY_COMMIT" ]]; then
            # Check if there are staged or unstaged changes to opencode docs/code
            # that haven't been reflected in HISTORY.md
            
            # Get files changed since last HISTORY.md update
            DOCS_CHANGED=$(git diff --name-only "$LAST_HISTORY_COMMIT" -- "$DOCS_DIR" 2>/dev/null | grep -v "HISTORY.md" | wc -l | tr -d ' ')
            CODE_CHANGED=$(git diff --name-only "$LAST_HISTORY_COMMIT" -- "$CODE_DIR" 2>/dev/null | wc -l | tr -d ' ')
            
            # Also check staged but uncommitted changes
            STAGED_DOCS=$(git diff --cached --name-only -- "$DOCS_DIR" 2>/dev/null | grep -v "HISTORY.md" | wc -l | tr -d ' ')
            STAGED_CODE=$(git diff --cached --name-only -- "$CODE_DIR" 2>/dev/null | wc -l | tr -d ' ')
            
            TOTAL_CHANGES=$((DOCS_CHANGED + CODE_CHANGED + STAGED_DOCS + STAGED_CODE))
            
            if [[ "$TOTAL_CHANGES" -gt 0 ]]; then
                # Check if HISTORY.md was also updated in the same set of changes
                HISTORY_IN_STAGED=$(git diff --cached --name-only -- "$HISTORY" 2>/dev/null | wc -l | tr -d ' ')
                
                if [[ "$HISTORY_IN_STAGED" -gt 0 ]]; then
                    success "HISTORY.md is staged with other changes"
                else
                    warn "OpenCode docs/code changed ($TOTAL_CHANGES files) since last HISTORY.md update"
                    info "Consider adding an entry to HISTORY.md before committing"
                    
                    # Show what changed
                    if [[ "$DOCS_CHANGED" -gt 0 ]]; then
                        info "  Docs changed: $DOCS_CHANGED files"
                    fi
                    if [[ "$CODE_CHANGED" -gt 0 ]]; then
                        info "  Code changed: $CODE_CHANGED files"
                    fi
                fi
            else
                success "HISTORY.md is current (no changes since last update)"
            fi
        else
            # HISTORY.md exists but no git history (new file)
            success "HISTORY.md exists (no git history yet)"
        fi
    else
        error "HISTORY.md not found"
    fi
    echo ""
fi

# ─────────────────────────────────────────────────────────────────
# 4. Check README inventories
# ─────────────────────────────────────────────────────────────────
if [[ "$CHECK_README" == true ]]; then
    echo "Checking README inventories..."
    
    # Check design/README.md lists all design docs
    DESIGN_README="$DOCS_DIR/design/README.md"
    if [[ -f "$DESIGN_README" ]]; then
        DESIGN_DOCS=$(find "$DOCS_DIR/design" -maxdepth 1 -name "*.md" ! -name "README.md" -type f | wc -l | tr -d ' ')
        DESIGN_LISTED=$(grep -c "\.md\]" "$DESIGN_README" 2>/dev/null || echo "0")
        
        if [[ "$DESIGN_DOCS" -gt "$DESIGN_LISTED" ]]; then
            warn "design/README.md may be missing entries (found $DESIGN_DOCS docs, $DESIGN_LISTED listed)"
        else
            success "design/README.md inventory looks complete"
        fi
    fi
    
    echo ""
fi

# ─────────────────────────────────────────────────────────────────
# 5. Check for missing frontmatter
# ─────────────────────────────────────────────────────────────────
if [[ "$QUICK" == false ]]; then
    echo "Checking frontmatter..."
    
    MISSING_PURPOSE=0
    while IFS= read -r file; do
        # Skip README files (they have different format)
        if [[ "$file" =~ README\.md$ ]]; then
            continue
        fi
        
        # Check for Purpose in first 10 lines
        if ! head -10 "$file" | grep -qi "purpose"; then
            warn "Missing Purpose in frontmatter: $file"
            ((MISSING_PURPOSE++))
        fi
    done < <(find "$DOCS_DIR/reference" -name "*.md" -type f)
    
    if [[ $MISSING_PURPOSE -eq 0 ]]; then
        success "All reference docs have Purpose frontmatter"
    fi
    echo ""
fi

# ─────────────────────────────────────────────────────────────────
# Summary
# ─────────────────────────────────────────────────────────────────
echo "═══════════════════════════════════════════════════════════"
if [[ $ERRORS -gt 0 ]]; then
    echo -e "${RED}FAILED${NC}: $ERRORS errors, $WARNINGS warnings"
    echo ""
    echo "See $CONTRIBUTING for documentation standards."
    exit 1
elif [[ $WARNINGS -gt 0 ]]; then
    echo -e "${YELLOW}PASSED${NC} with $WARNINGS warnings"
    echo ""
    echo "Review warnings and update docs if needed."
    echo "See $CONTRIBUTING for guidance."
    exit 0
else
    echo -e "${GREEN}PASSED${NC}: All checks passed"
    exit 0
fi
