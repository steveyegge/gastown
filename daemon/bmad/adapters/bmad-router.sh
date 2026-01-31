#!/bin/bash
# Gastown - BMad Router
# Главный роутер для интеграции BMad в dispatch.sh
# Level 0-2 → Quick Flow, Level 3-4 → Full Workflow
# Author: Gastown Contributors
# Версия: 2.0 (Production)

set -e

# Директории
GT_DIR="$HOME/gt"
DAEMON_DIR="$GT_DIR/daemon"
BMAD_ADAPTERS="$DAEMON_DIR/bmad/adapters"
LEARNING_DIR="$DAEMON_DIR/learning"
RESULTS_TRACKER="$LEARNING_DIR/results-tracker.sh"

# Цвета
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

log() { echo -e "${GREEN}[BMAD-ROUTER]${NC} $1" >&2; }
info() { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1" >&2; }

# Проверка зависимостей
check_dependencies() {
    local all_ok=true

    if [ ! -x "$BMAD_ADAPTERS/detect-scale.sh" ]; then
        error "detect-scale.sh не найден или не исполняемый"
        all_ok=false
    fi

    if [ ! -x "$BMAD_ADAPTERS/quick-flow-adapter.sh" ]; then
        error "quick-flow-adapter.sh не найден или не исполняемый"
        all_ok=false
    fi

    if [ ! -x "$BMAD_ADAPTERS/bmad-full-workflow.sh" ]; then
        error "bmad-full-workflow.sh не найден или не исполняемый"
        all_ok=false
    fi

    if [ "$all_ok" = false ]; then
        return 1
    fi

    return 0
}

# Определение масштаба задачи
detect_task_scale() {
    local description="$1"

    info "Определяю масштаб задачи..."
    local scale_result=$("$BMAD_ADAPTERS/detect-scale.sh" "$description" 2>/dev/null)

    if [ -z "$scale_result" ]; then
        error "Не удалось определить масштаб задачи"
        return 1
    fi

    echo "$scale_result"
}

# Роутинг задачи
route_task() {
    local task_id="$1"
    local description="$2"
    local scale_json="$3"

    # Извлекаем уровень из JSON
    local level=$(echo "$scale_json" | grep -o '"level": [0-9]*' | grep -o '[0-9]*')
    local track=$(echo "$scale_json" | grep -o '"track_recommendation": "[^"]*"' | cut -d'"' -f4)

    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Routing Decision"
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Task: $task_id"
    log "Level: $level"
    log "Track: $track"
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    local result=""

    if [ "$track" = "quick-flow" ]; then
        # Level 0-2: Quick Flow
        info "Маршрут: Quick Flow Adapter"
        result=$("$BMAD_ADAPTERS/quick-flow-adapter.sh" "$task_id" "$level" "$description")

    else
        # Level 3-4: Full Workflow
        info "Маршрут: Full Workflow (Level 3-4)"
        result=$("$BMAD_ADAPTERS/bmad-full-workflow.sh" "$task_id" "$level" "$description")
    fi

    echo "$result"
}

# Сохранение метрик
save_metrics() {
    local task_id="$1"
    local result_json="$2"

    if [ -x "$RESULTS_TRACKER" ]; then
        # Сохраняем результат через results-tracker
        echo "$result_json" | "$RESULTS_TRACKER" record bmad "$task_id" 2>/dev/null || true
    fi
}

# === MAIN ===

# Использование
if [ $# -lt 2 ]; then
    cat >&2 <<EOF
${RED}Использование:${NC} $0 <task-id> <description>

${BLUE}Описание:${NC}
  Главный роутер для интеграции BMad в Gastown dispatch.
  Определяет сложность задачи и выбирает подходящий путь обработки.

${BLUE}Пути обработки:${NC}
  Level 0-2: Quick Flow (fast, focused workflows)
  Level 3-4: BMad Method (full development lifecycle)

${BLUE}Примеры:${NC}
  $0 task-001 "Fix typo in README"
  $0 task-002 "Add new feature to authentication"
  $0 task-003 "Design microservices architecture"

EOF
    exit 1
fi

TASK_ID="$1"
shift
DESCRIPTION="$*"

log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log "BMad Router v2.0 - Production Ready"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Проверяем зависимости
if ! check_dependencies; then
    error "Не все зависимости доступны"
    exit 1
fi

# Определяем масштаб
SCALE_JSON=$(detect_task_scale "$DESCRIPTION")
if [ $? -ne 0 ]; then
    error "Ошибка при определении масштаба"
    exit 1
fi

info "Масштаб определён:"
echo "$SCALE_JSON" | grep -v "^$" | head -5 >&2

# Роутим задачу
RESULT=$(route_task "$TASK_ID" "$DESCRIPTION" "$SCALE_JSON")

# Сохраняем метрики
save_metrics "$TASK_ID" "$RESULT"

# Выводим результат в stdout
echo "$RESULT"

success "Роутинг завершён"
exit 0
