#!/bin/bash
# Gastown - BMad Integration Test Examples
# Тестовые примеры для проверки работы BMad Router
# Версия: 1.0 (Proof of Concept)

set -e

GT_DIR="$HOME/gt"
DAEMON_DIR="$GT_DIR/daemon"
BMAD_ROUTER="$DAEMON_DIR/bmad/adapters/bmad-router.sh"

# Цвета
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${CYAN}[TEST]${NC} $1"; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

echo ""
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log "BMad Integration Tests - Proof of Concept"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Проверка существования роутера
if [ ! -x "$BMAD_ROUTER" ]; then
    error "BMad Router не найден или не исполняемый: $BMAD_ROUTER"
    exit 1
fi

# === ТЕСТ 1: Level 0 (Документация) ===
echo ""
log "╔════════════════════════════════════════════════════╗"
log "║ ТЕСТ 1: Level 0 - Тривиальная задача              ║"
log "╚════════════════════════════════════════════════════╝"
echo ""

TEST1_DESCRIPTION="Fix typo in README documentation"
info "Описание: $TEST1_DESCRIPTION"
echo ""

TEST1_RESULT=$("$BMAD_ROUTER" "test-task-001" "$TEST1_DESCRIPTION" 2>&1)
TEST1_EXIT=$?

echo ""
echo "$TEST1_RESULT" | grep -v "^\[" | head -10
echo ""

if [ $TEST1_EXIT -eq 0 ]; then
    success "ТЕСТ 1 PASSED ✓"
else
    error "ТЕСТ 1 FAILED ✗"
fi

sleep 2

# === ТЕСТ 2: Level 1 (Bug Fix) ===
echo ""
log "╔════════════════════════════════════════════════════╗"
log "║ ТЕСТ 2: Level 1 - Простая задача (Bug Fix)        ║"
log "╚════════════════════════════════════════════════════╝"
echo ""

TEST2_DESCRIPTION="Fix bug in user login validation"
info "Описание: $TEST2_DESCRIPTION"
echo ""

TEST2_RESULT=$("$BMAD_ROUTER" "test-task-002" "$TEST2_DESCRIPTION" 2>&1)
TEST2_EXIT=$?

echo ""
echo "$TEST2_RESULT" | grep -v "^\[" | head -10
echo ""

if [ $TEST2_EXIT -eq 0 ]; then
    success "ТЕСТ 2 PASSED ✓"
else
    error "ТЕСТ 2 FAILED ✗"
fi

sleep 2

# === ТЕСТ 3: Level 2 (New Feature) ===
echo ""
log "╔════════════════════════════════════════════════════╗"
log "║ ТЕСТ 3: Level 2 - Средняя задача (New Feature)    ║"
log "╚════════════════════════════════════════════════════╝"
echo ""

TEST3_DESCRIPTION="Add new feature authentication with OAuth integration"
info "Описание: $TEST3_DESCRIPTION"
echo ""

TEST3_RESULT=$("$BMAD_ROUTER" "test-task-003" "$TEST3_DESCRIPTION" 2>&1)
TEST3_EXIT=$?

echo ""
echo "$TEST3_RESULT" | grep -v "^\[" | head -10
echo ""

if [ $TEST3_EXIT -eq 0 ]; then
    success "ТЕСТ 3 PASSED ✓"
else
    error "ТЕСТ 3 FAILED ✗"
fi

sleep 2

# === ТЕСТ 4: Level 3 (Complex - должен вернуть deferred) ===
echo ""
log "╔════════════════════════════════════════════════════╗"
log "║ ТЕСТ 4: Level 3 - Сложная задача (Deferred)       ║"
log "╚════════════════════════════════════════════════════╝"
echo ""

TEST4_DESCRIPTION="Design and implement microservices module architecture"
info "Описание: $TEST4_DESCRIPTION"
echo ""

TEST4_RESULT=$("$BMAD_ROUTER" "test-task-004" "$TEST4_DESCRIPTION" 2>&1)
TEST4_EXIT=$?

echo ""
echo "$TEST4_RESULT" | grep -v "^\[" | head -10
echo ""

if [ $TEST4_EXIT -eq 0 ]; then
    success "ТЕСТ 4 PASSED ✓ (ожидаемо deferred)"
else
    error "ТЕСТ 4 FAILED ✗"
fi

# === ИТОГИ ===
echo ""
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log "Результаты тестирования"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

PASSED=0
FAILED=0

[ $TEST1_EXIT -eq 0 ] && PASSED=$((PASSED+1)) || FAILED=$((FAILED+1))
[ $TEST2_EXIT -eq 0 ] && PASSED=$((PASSED+1)) || FAILED=$((FAILED+1))
[ $TEST3_EXIT -eq 0 ] && PASSED=$((PASSED+1)) || FAILED=$((FAILED+1))
[ $TEST4_EXIT -eq 0 ] && PASSED=$((PASSED+1)) || FAILED=$((FAILED+1))

echo ""
info "Всего тестов: 4"
success "Успешно: $PASSED"
[ $FAILED -gt 0 ] && error "Провалено: $FAILED" || true
echo ""

if [ $FAILED -eq 0 ]; then
    success "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    success "ВСЕ ТЕСТЫ ПРОЙДЕНЫ ✓"
    success "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    error "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    error "НЕКОТОРЫЕ ТЕСТЫ НЕ ПРОЙДЕНЫ ✗"
    error "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 1
fi
