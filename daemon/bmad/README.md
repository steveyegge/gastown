# BMad Integration for Gastown

**Intelligent task routing using BMad (Breakthrough Method of Agile AI-driven Development)**

Version: 2.0
Date: 2026-01-23

---

## Overview

BMad integration adds intelligent task complexity detection and adaptive routing to Gastown. Tasks are automatically classified by complexity level (0-4) and routed to appropriate workflows:

- **Level 0-2**: Quick Flow (single agent, fast execution)
- **Level 3-4**: Full Workflow (multi-agent team, phased execution)

### Architecture

```
dispatch.sh create "task description"
       |
bmad_route_task() - complexity detection (Level 0-4)
       |
bmad-router.sh - routing
       |
       +-- Level 0-2: quick-flow-adapter.sh (2-3s)
       |              Single agent execution
       |
       +-- Level 3-4: bmad-full-workflow.sh (5-6s)
                      Multi-agent team, 4-phase execution
       |
process-artifacts.sh - artifact processing
       |
artifacts/bmad/ - structured storage
```

---

## Quick Start

### Creating Tasks

```bash
# Simple task (Level 0-2) - Quick Flow
./dispatch.sh create "Fix typo in README"
# -> debugger agent, 2s

# Complex task (Level 3-4) - Full Workflow
./dispatch.sh create "Design enterprise microservices architecture"
# -> cloud-architect + backend-architect + code-reviewer, 5s
```

### Direct BMad Router Call

```bash
cd daemon/bmad/adapters

# Quick Flow
./bmad-router.sh "task-001" "Add form validation"

# Full Workflow
./bmad-router.sh "task-002" "Architect distributed event-driven system"
```

---

## Components

### 1. detect-scale.sh

Detects task complexity (Level 0-4) based on keywords.

```bash
./detect-scale.sh "Fix typo"
# -> Level 0, score 5, track: quick-flow

./detect-scale.sh "Design microservices with Kubernetes"
# -> Level 4, score 130, track: bmad-method
```

**Complexity Markers:**
- Level 0 (0-15): typo, comment, readme
- Level 1 (16-30): fix, bug, update, add
- Level 2 (31-60): refactor, implement, feature
- Level 3 (61-100): module, component, integration
- Level 4 (100+): architect, enterprise, distributed, microservices

### 2. bmad-router.sh

Main router - directs tasks to Quick Flow or Full Workflow.

```bash
./bmad-router.sh <task-id> <description>
```

**Output:** JSON with execution result

### 3. quick-flow-adapter.sh

Fast execution for simple tasks (Level 0-2).

**Agents by level:**
- Level 0: general-purpose
- Level 1: debugger (bugs), test-automator (tests), general-purpose
- Level 2: frontend-developer (UI), python-pro (refactor), test-automator (tests)

**Execution time:** 2-3 seconds

### 4. bmad-full-workflow.sh

4-phase execution for complex tasks (Level 3-4).

**Phases:**
1. **Analysis** (1s) -> technical-design.md
2. **Implementation** (2s) -> implementation.md
3. **Quality Check** (1s) -> quality-report.md
4. **Finalization** (1s) -> execution-plan.md

**Agents by level:**
- Level 3: python-pro + code-reviewer
- Level 4 (architecture): cloud-architect + backend-architect + code-reviewer
- Level 4 (data): database-optimizer + data-engineer + code-reviewer

**Execution time:** 5-6 seconds

### 5. process-artifacts.sh

Automatic artifact processing and storage.

```bash
# Process artifacts
./process-artifacts.sh process /tmp/bmad-artifacts-task-001 task-001

# Show statistics
./process-artifacts.sh stats

# Clean old artifacts (>24h)
./process-artifacts.sh cleanup
```

---

## Storage Structure

```
daemon/artifacts/bmad/
├── quick-flow/
│   ├── implementations/     # Quick Flow results
│   │   └── {task-id}-output.md
│   └── metadata/            # Metadata
│       └── {task-id}-meta.json
├── full-workflow/
│   ├── plans/               # Execution plans
│   │   └── {task-id}-plan.md
│   ├── designs/             # Technical designs
│   │   └── {task-id}-design.md
│   ├── implementations/     # Implementation
│   │   └── {task-id}-impl.md
│   ├── quality/             # Quality reports
│   │   └── {task-id}-quality.md
│   └── metadata/            # Metadata
│       └── {task-id}-meta.json
└── archive/                 # Source archive
    └── YYYYMMDD/
        └── {task-id}/
```

---

## Performance

### Benchmark Results

| Workflow | Level | Execution Time | Agents |
|----------|-------|----------------|--------|
| Quick Flow | 0 | 2s | 1 |
| Quick Flow | 1 | 2s | 1 |
| Quick Flow | 2 | 2-3s | 1 |
| Full Workflow | 3 | 5s | 2 |
| Full Workflow | 4 | 5-6s | 3 |

### Stress Test

5 parallel tasks execute without conflicts:
- No race conditions
- Artifacts don't overlap
- Metadata is correct

---

## Integration with dispatch.sh

To enable BMad routing in dispatch.sh, add the following:

```bash
# Add to dispatch.sh
BMAD_ROUTER="$DAEMON_DIR/bmad/adapters/bmad-router.sh"
BMAD_DETECTOR="$DAEMON_DIR/bmad/adapters/detect-scale.sh"

bmad_route_task() {
    local description="$1"
    local detection=$("$BMAD_DETECTOR" "$description" 2>/dev/null)
    local level=$(echo "$detection" | grep "level:" | cut -d: -f2 | tr -d ' ')

    if [ "$level" -le 2 ]; then
        echo "agent:bmad-quick-flow"
    else
        echo "agent:bmad-full-workflow"
    fi
}
```

---

## Configuration

Edit `config/gastown-bmad.yaml`:

```yaml
gastown_integration:
  enabled: true
  priority: "auto"
  fallback_to_standard: true
  save_metrics: true

routing:
  quick_flow_threshold: 2
  ml_router_threshold: 3
```

---

## Troubleshooting

### Task detected as low Level

**Solution:** Add explicit complexity markers:
- "enterprise" (+45)
- "distributed" (+30)
- "microservices" (+40)
- "architect" (+50)

### Artifacts not processing

1. Check permissions: `chmod +x process-artifacts.sh`
2. Initialize: `./process-artifacts.sh init`

---

## Files

```
daemon/bmad/
├── adapters/
│   ├── bmad-router.sh          # Main router
│   ├── detect-scale.sh         # Complexity detection
│   ├── quick-flow-adapter.sh   # Quick Flow (Level 0-2)
│   ├── bmad-full-workflow.sh   # Full Workflow (Level 3-4)
│   ├── process-artifacts.sh    # Artifact processing
│   └── test-examples.sh        # Test suite
├── config/
│   └── gastown-bmad.yaml       # Configuration
└── README.md                   # This documentation
```

---

## Version History

### v2.0 (2026-01-23) - Production
- Full Workflow for Level 3-4
- dispatch.sh integration
- Automatic artifact processing
- Structured storage
- Stress test passed

### v1.0 (2026-01-22) - PoC
- Quick Flow (simulation)
- Basic routing

---

*Gastown + BMad Integration v2.0*
