#!/bin/bash
# Gastown - BMad Scale Detector
# Определяет сложность задачи (Level 0-4) для выбора правильного BMad workflow
# Author: Gastown Contributors
# Версия: 1.0 (Proof of Concept)

set -e

# Цвета для логов
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${CYAN}[SCALE-DETECT]${NC} $1" >&2; }
info() { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

# Функция определения уровня сложности
# Level 0: Тривиальные задачи (документация, мелкие исправления)
# Level 1: Простые задачи (bug fix, small feature)
# Level 2: Средние задачи (new feature, refactoring)
# Level 3: Сложные задачи (module, integration)
# Level 4: Архитектурные задачи (system design, platform)
detect_scale_level() {
    local description="$1"
    local level=0
    local score=0

    # Преобразуем в нижний регистр для анализа
    local desc_lower=$(echo "$description" | tr '[:upper:]' '[:lower:]')

    # === УРОВЕНЬ 0: Тривиальные задачи (0-10 баллов) ===
    if echo "$desc_lower" | grep -qE "(docs?|documentation|readme|comment|typo|format|style)"; then
        score=$((score + 2))
        info "Найден маркер документации (+2)"
    fi

    # === УРОВЕНЬ 1: Простые задачи (11-30 баллов) ===
    if echo "$desc_lower" | grep -qE "(fix|bug|error|issue|patch|hotfix)"; then
        score=$((score + 15))
        info "Найден маркер bug fix (+15)"
    fi

    if echo "$desc_lower" | grep -qE "(small|minor|quick|simple)"; then
        score=$((score + 8))
        info "Найден маркер простоты (+8)"
    fi

    # === УРОВЕНЬ 2: Средние задачи (31-60 баллов) ===
    if echo "$desc_lower" | grep -qE "(feature|add|implement|create|new)"; then
        score=$((score + 25))
        info "Найден маркер новой функциональности (+25)"
    fi

    if echo "$desc_lower" | grep -qE "(refactor|improve|optimize|enhance)"; then
        score=$((score + 20))
        info "Найден маркер рефакторинга (+20)"
    fi

    if echo "$desc_lower" | grep -qE "(test|testing|unit|integration)"; then
        score=$((score + 15))
        info "Найден маркер тестирования (+15)"
    fi

    # === УРОВЕНЬ 3: Сложные задачи (61-90 баллов) ===
    if echo "$desc_lower" | grep -qE "(module|component|service|api|endpoint)"; then
        score=$((score + 40))
        info "Найден маркер модуля/компонента (+40)"
    fi

    if echo "$desc_lower" | grep -qE "(integration|connect|bridge|adapter)"; then
        score=$((score + 35))
        info "Найден маркер интеграции (+35)"
    fi

    if echo "$desc_lower" | grep -qE "(database|schema|migration|model)"; then
        score=$((score + 30))
        info "Найден маркер работы с БД (+30)"
    fi

    # === УРОВЕНЬ 4: Архитектурные задачи (90+ баллов) ===
    if echo "$desc_lower" | grep -qE "(architecture|design|platform|system|framework)"; then
        score=$((score + 50))
        info "Найден маркер архитектуры (+50)"
    fi

    if echo "$desc_lower" | grep -qE "(enterprise|scalable|distributed|microservice)"; then
        score=$((score + 45))
        info "Найден маркер enterprise (+45)"
    fi

    # Определяем уровень по набранным баллам
    if [ $score -le 10 ]; then
        level=0
    elif [ $score -le 30 ]; then
        level=1
    elif [ $score -le 60 ]; then
        level=2
    elif [ $score -le 90 ]; then
        level=3
    else
        level=4
    fi

    log "Описание: '$description'"
    log "Набрано баллов: $score"
    log "Определён уровень: $level"

    # Возвращаем JSON
    cat <<EOF
{
    "level": $level,
    "score": $score,
    "description": "$description",
    "track_recommendation": "$([ $level -le 2 ] && echo "quick-flow" || echo "bmad-method")"
}
EOF
}

# === MAIN ===

if [ $# -eq 0 ]; then
    error "Использование: $0 <task-description>"
    error "Пример: $0 'Fix bug in login form'"
    exit 1
fi

TASK_DESCRIPTION="$*"
detect_scale_level "$TASK_DESCRIPTION"
