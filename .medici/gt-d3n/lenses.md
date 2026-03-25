# Medici Lenses: E-stop (gt-d3n)

## Lens Set (6 lenses)

### 1. domain — Industrial control systems
**Question**: How do real factory E-stops work, and what design principles transfer to software agent orchestration?
**Blind spot**: May over-engineer for hardware reliability guarantees that don't apply to software.
**Output**: `.medici/gt-d3n/domain.md`

### 2. user — The overseer under stress
**Question**: When the overseer sees agents failing, what do they actually need in that moment — and what would make them trust an automatic system?
**Blind spot**: May optimize for the human's comfort at the expense of system-level efficiency.
**Output**: `.medici/gt-d3n/user.md`

### 3. constraints — Gas Town architecture realities
**Question**: Given daemon, tmux, Dolt, file-based IPC, and existing rig lifecycle, what's the simplest reliable signal path that works even when the failure IS the infrastructure?
**Blind spot**: May be too conservative, only proposing what's easy to build with current primitives.
**Output**: `.medici/gt-d3n/constraints.md`

### 4. incentives — Agent economics and token waste
**Question**: What's the actual cost model of agents working during failures, and where is the threshold between "let them retry" and "kill everything"?
**Blind spot**: May reduce the problem to pure cost optimization and miss reliability/safety aspects.
**Output**: `.medici/gt-d3n/incentives.md`

### 5. adversary — Failure modes of the E-stop itself
**Question**: How can the E-stop mechanism itself fail, be triggered falsely, or make things worse?
**Blind spot**: May be paralyzed by edge cases and recommend doing nothing.
**Output**: `.medici/gt-d3n/adversary.md`

### 6. outsider — Circuit breakers in distributed systems
**Question**: How do microservice circuit breakers, Kubernetes pod disruption budgets, and chaos engineering handle this — and what's the minimal viable pattern?
**Blind spot**: May import complexity from systems operating at very different scale.
**Output**: `.medici/gt-d3n/outsider.md`
