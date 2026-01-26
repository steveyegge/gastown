#!/bin/bash
# Gastown - BMad Artifacts Processor
# Обработка и сохранение артефактов из workflows
# Author: Gastown Contributors
# Версия: 1.0

set -e

# Директории
GT_DIR="$HOME/gt"
DAEMON_DIR="$GT_DIR/daemon"
ARTIFACTS_BASE="$DAEMON_DIR/artifacts"
BMAD_ARTIFACTS="$ARTIFACTS_BASE/bmad"

# Цвета
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

log() { echo -e "${CYAN}[ARTIFACTS]${NC} $1" >&2; }
info() { echo -e "${BLUE}[INFO]${NC} $1" >&2; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1" >&2; }

# Создать структуру директорий
init_structure() {
    log "Инициализация структуры артефактов..."

    mkdir -p "$BMAD_ARTIFACTS"/{quick-flow,full-workflow}/{plans,designs,implementations,quality,metadata}
    mkdir -p "$ARTIFACTS_BASE/archive"

    success "Структура создана"
}

# Определить тип workflow по артефактам
detect_workflow_type() {
    local source_dir="$1"

    # Quick Flow артефакты содержат output.md
    if [ -f "$source_dir/output.md" ]; then
        echo "quick-flow"
        return 0
    fi

    # Full Workflow артефакты содержат 4 файла
    if [ -f "$source_dir/execution-plan.md" ] && \
       [ -f "$source_dir/technical-design.md" ] && \
       [ -f "$source_dir/implementation.md" ] && \
       [ -f "$source_dir/quality-report.md" ]; then
        echo "full-workflow"
        return 0
    fi

    # Неизвестный тип
    echo "unknown"
}

# Обработать артефакты Quick Flow
process_quick_flow() {
    local source_dir="$1"
    local task_id="$2"
    local target_dir="$BMAD_ARTIFACTS/quick-flow"

    log "Обработка Quick Flow артефактов для $task_id"

    # Копировать output.md
    if [ -f "$source_dir/output.md" ]; then
        cp "$source_dir/output.md" "$target_dir/implementations/${task_id}-output.md"
        info "✓ output.md → implementations/"
    fi

    # Создать метаданные
    cat > "$target_dir/metadata/${task_id}-meta.json" <<EOF
{
    "task_id": "$task_id",
    "workflow_type": "quick-flow",
    "processed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "source": "$source_dir",
    "artifacts": [
        "implementations/${task_id}-output.md"
    ]
}
EOF

    success "Quick Flow артефакты обработаны"
}

# Обработать артефакты Full Workflow
process_full_workflow() {
    local source_dir="$1"
    local task_id="$2"
    local target_dir="$BMAD_ARTIFACTS/full-workflow"

    log "Обработка Full Workflow артефактов для $task_id"

    local artifacts=()

    # Копировать execution-plan.md
    if [ -f "$source_dir/execution-plan.md" ]; then
        cp "$source_dir/execution-plan.md" "$target_dir/plans/${task_id}-plan.md"
        info "✓ execution-plan.md → plans/"
        artifacts+=("plans/${task_id}-plan.md")
    fi

    # Копировать technical-design.md
    if [ -f "$source_dir/technical-design.md" ]; then
        cp "$source_dir/technical-design.md" "$target_dir/designs/${task_id}-design.md"
        info "✓ technical-design.md → designs/"
        artifacts+=("designs/${task_id}-design.md")
    fi

    # Копировать implementation.md
    if [ -f "$source_dir/implementation.md" ]; then
        cp "$source_dir/implementation.md" "$target_dir/implementations/${task_id}-impl.md"
        info "✓ implementation.md → implementations/"
        artifacts+=("implementations/${task_id}-impl.md")
    fi

    # Копировать quality-report.md
    if [ -f "$source_dir/quality-report.md" ]; then
        cp "$source_dir/quality-report.md" "$target_dir/quality/${task_id}-quality.md"
        info "✓ quality-report.md → quality/"
        artifacts+=("quality/${task_id}-quality.md")
    fi

    # Создать метаданные
    local artifacts_json=$(printf ',"%s"' "${artifacts[@]}")
    artifacts_json="[${artifacts_json:1}]"

    cat > "$target_dir/metadata/${task_id}-meta.json" <<EOF
{
    "task_id": "$task_id",
    "workflow_type": "full-workflow",
    "processed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "source": "$source_dir",
    "artifacts": $artifacts_json,
    "phases": {
        "analysis": "completed",
        "implementation": "completed",
        "quality_check": "completed",
        "finalization": "completed"
    }
}
EOF

    success "Full Workflow артефакты обработаны (${#artifacts[@]} файлов)"
}

# Архивировать исходные артефакты
archive_source() {
    local source_dir="$1"
    local task_id="$2"

    local archive_dir="$ARTIFACTS_BASE/archive/$(date +%Y%m%d)"
    mkdir -p "$archive_dir"

    # Переместить в архив
    if [ -d "$source_dir" ]; then
        mv "$source_dir" "$archive_dir/${task_id}"
        info "✓ Исходники архивированы → $archive_dir/${task_id}"
    fi
}

# Обработать артефакты
process_artifacts() {
    local source_dir="$1"
    local task_id="$2"
    local keep_source="${3:-false}"  # По умолчанию удаляем исходники

    if [ ! -d "$source_dir" ]; then
        error "Директория не найдена: $source_dir"
        return 1
    fi

    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Обработка артефактов"
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Source: $source_dir"
    log "Task ID: $task_id"
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    # Инициализировать структуру если нужно
    init_structure

    # Определить тип workflow
    local workflow_type=$(detect_workflow_type "$source_dir")
    info "Тип workflow: $workflow_type"

    # Обработать согласно типу
    case "$workflow_type" in
        quick-flow)
            process_quick_flow "$source_dir" "$task_id"
            ;;
        full-workflow)
            process_full_workflow "$source_dir" "$task_id"
            ;;
        unknown)
            warn "Неизвестный тип артефактов, копирую как есть"
            cp -r "$source_dir" "$ARTIFACTS_BASE/archive/unknown-${task_id}"
            ;;
    esac

    # Архивировать или удалить исходники
    if [ "$keep_source" = "true" ]; then
        info "Исходники сохранены: $source_dir"
    else
        archive_source "$source_dir" "$task_id"
    fi

    success "Обработка завершена"
}

# Показать статистику артефактов
show_stats() {
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "Статистика артефактов BMad"
    log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Quick Flow
    local qf_impl=$(find "$BMAD_ARTIFACTS/quick-flow/implementations" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    local qf_meta=$(find "$BMAD_ARTIFACTS/quick-flow/metadata" -name "*.json" 2>/dev/null | wc -l | tr -d ' ')

    echo -e "${GREEN}Quick Flow:${NC}"
    echo "  Implementations: $qf_impl"
    echo "  Metadata: $qf_meta"
    echo ""

    # Full Workflow
    local fw_plans=$(find "$BMAD_ARTIFACTS/full-workflow/plans" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    local fw_designs=$(find "$BMAD_ARTIFACTS/full-workflow/designs" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    local fw_impl=$(find "$BMAD_ARTIFACTS/full-workflow/implementations" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    local fw_quality=$(find "$BMAD_ARTIFACTS/full-workflow/quality" -name "*.md" 2>/dev/null | wc -l | tr -d ' ')
    local fw_meta=$(find "$BMAD_ARTIFACTS/full-workflow/metadata" -name "*.json" 2>/dev/null | wc -l | tr -d ' ')

    echo -e "${GREEN}Full Workflow:${NC}"
    echo "  Plans: $fw_plans"
    echo "  Designs: $fw_designs"
    echo "  Implementations: $fw_impl"
    echo "  Quality Reports: $fw_quality"
    echo "  Metadata: $fw_meta"
    echo ""

    # Архив
    local archived=$(find "$ARTIFACTS_BASE/archive" -type d -mindepth 1 2>/dev/null | wc -l | tr -d ' ')
    echo -e "${YELLOW}Архив:${NC}"
    echo "  Директорий: $archived"
    echo ""

    # Общая статистика
    local total=$((qf_impl + fw_plans + fw_designs + fw_impl + fw_quality))
    echo -e "${CYAN}Итого артефактов:${NC} $total"
}

# Очистить временные артефакты в /tmp
cleanup_temp() {
    log "Очистка временных артефактов в /tmp..."

    local cleaned=0

    # Найти и удалить старые артефакты (> 24 часа)
    while IFS= read -r dir; do
        if [ -d "$dir" ]; then
            rm -rf "$dir"
            ((cleaned++))
        fi
    done < <(find /tmp -maxdepth 1 -type d -name "bmad-*-artifacts-*" -mtime +0 2>/dev/null)

    success "Удалено старых артефактов: $cleaned"
}

# Показать справку
show_help() {
    cat >&2 <<EOF
${GREEN}BMad Artifacts Processor${NC}

${BLUE}Использование:${NC}
    $0 process <source_dir> <task_id> [keep_source]
    $0 stats
    $0 cleanup
    $0 init
    $0 help

${BLUE}Команды:${NC}
    process <dir> <id> [keep]  - Обработать артефакты
    stats                      - Показать статистику
    cleanup                    - Очистить /tmp от старых артефактов
    init                       - Инициализировать структуру
    help                       - Показать эту справку

${BLUE}Примеры:${NC}
    # Обработать и архивировать артефакты
    $0 process /tmp/bmad-artifacts-task-001 task-001

    # Обработать но сохранить исходники
    $0 process /tmp/bmad-artifacts-task-001 task-001 true

    # Показать статистику
    $0 stats

    # Очистить временные файлы
    $0 cleanup

${BLUE}Структура:${NC}
    artifacts/bmad/
    ├── quick-flow/
    │   ├── implementations/    - Результаты выполнения
    │   └── metadata/           - Метаданные задач
    ├── full-workflow/
    │   ├── plans/              - Планы выполнения
    │   ├── designs/            - Технические дизайны
    │   ├── implementations/    - Реализация
    │   ├── quality/            - Отчёты о качестве
    │   └── metadata/           - Метаданные задач
    └── archive/                - Архив исходников

EOF
}

# === MAIN ===

case "${1:-help}" in
    process)
        if [ $# -lt 3 ]; then
            error "Использование: $0 process <source_dir> <task_id> [keep_source]"
            exit 1
        fi
        process_artifacts "$2" "$3" "${4:-false}"
        ;;

    stats)
        show_stats
        ;;

    cleanup)
        cleanup_temp
        ;;

    init)
        init_structure
        ;;

    help|*)
        show_help
        ;;
esac

exit 0
