# bv Analysis: gt-csl Bead Graph (2026-02-20)

Raw data: `bv-insights.json` (82KB, `bv --robot-insights --format json`)

## Health Check

| Check | Result |
|-------|--------|
| `bd dep cycles` | No cycles |
| `bv --robot-alerts` | 0 alerts involving gt-csl |
| `bv --robot-suggest --suggest-type cycle` | 0 cycle suggestions |
| `bv --robot-suggest --suggest-type dependency` | 0 missing dep suggestions |
| `bv --robot-suggest --suggest-type duplicate` | 0 duplicate suggestions |

## Graph Structure

- 31 beads: 1 root epic + 7 sub-epics + 21 tasks + 2 implicit
- 24 blocking dependencies
- 7 Wave 1 tasks (in-degree=0): `1.1`, `1.2`, `2.1`, `2.2`, `2.3`, `3.1`, `5.2`
- Critical path depth: 8 (2.3/3.1 → 3.2 → 3.4 → 3.5 → 5.3 → 6.1 → 6.2 → 6.3)

## PageRank (importance within gt-csl subgraph)

| Bead | PageRank | Role |
|------|----------|------|
| gt-csl.3.5 | 0.00695 | **Highest** — convergence point |
| gt-csl.3.1 | 0.00412 | Input parsing — feeds everything |
| gt-csl.3.2 | 0.00390 | DAG walking |
| gt-csl.2.2 | 0.00388 | computeWaves |
| gt-csl.2.3 | 0.00368 | buildDAG |
| gt-csl.6.1 | 0.00335 | Launch transition |
| gt-csl.2.1 | 0.00262 | detectCycles |
| gt-csl.3.3 | 0.00246 | Error detection |
| gt-csl.3.4 | 0.00246 | Warning detection |
| gt-csl.5.3 | 0.00241 | Status validation |
| gt-csl.6.2 | 0.00181 | Wave 1 dispatch |
| gt-csl.1.2 | 0.00140 | waveAssert |
| gt-csl.7.1 | 0.00140 | Random DAG gen |

## Critical Path Scores (downstream dependents)

| Bead | Score | Interpretation |
|------|-------|---------------|
| gt-csl.2.3 | 8 | Delay here delays 8 downstream tasks |
| gt-csl.3.1 | 8 | Same — tied for most critical |
| gt-csl.2.1 | 7 | |
| gt-csl.3.2 | 7 | |
| gt-csl.2.2 | 6 | |
| gt-csl.3.3 | 6 | |
| gt-csl.3.4 | 6 | |
| gt-csl.3.5 | 5 | The convergence bottleneck |
| gt-csl.5.3 | 4 | |
| gt-csl.6.1 | 3 | |
| gt-csl.6.2 | 2 | |

## HITS Analysis

### Top Hubs (depend on many important things)

| Bead | Hub Score |
|------|-----------|
| gt-csl.3.5 | 8.53e-7 |
| gt-csl.6.1 | 6.14e-7 |
| gt-csl.3.6 | 4.97e-7 |
| gt-csl.4.3 | 4.97e-7 |
| gt-csl.5.1 | 4.97e-7 |

### Top Authorities (depended on by many hubs)

| Bead | Authority Score |
|------|----------------|
| gt-csl.3.5 | 1.72e-6 |
| gt-csl.2.2 | 1.05e-6 |
| gt-csl.3.1 | 7.86e-7 |
| gt-csl.3.3 | 5.64e-7 |
| gt-csl.3.4 | 5.64e-7 |

gt-csl.3.5 is both top Hub AND top Authority — it's the central convergence/fan-out node.

## Articulation Points

Removing any of these disconnects the graph:

| Bead | Why |
|------|-----|
| gt-csl.2.2 | Only path to 4.2 (wave table) and 7.1 (DAG gen) |
| gt-csl.3.2 | Only path to 3.4 (warnings) and 4.1 (tree display) |
| gt-csl.3.5 | The bottleneck — all of epic 5/6 depends on it |
| gt-csl.6.1 | Only path to 6.2/6.3/6.4 |
| gt-csl.6.2 | Only path to 6.3 |
| gt-csl.7.1 | Only path to 7.2 |
| gt-csl.7.2 | Leaf of property test chain |

All expected — linear chains (6.1→6.2→6.3, 7.1→7.2) and single-path fan-outs are by design.

## Degree Distribution

| Bead | In | Out | Role |
|------|-----|-----|------|
| gt-csl.3.5 | 5 | 4 | **Highest both** — convergence + fan-out |
| gt-csl.6.1 | 2 | 2 | Launch gateway |
| gt-csl.3.2 | 2 | 2 | DAG walk gateway |
| gt-csl.2.2 | 3 | 0 | Pure sink (Wave 1, no deps) |
| gt-csl.3.1 | 2 | 0 | Pure sink (Wave 1, no deps) |
| gt-csl.2.3 | 2 | 0 | Pure sink (Wave 1, no deps) |

## What-If Analysis

| Close this... | Direct unblocks | Transitive unblocks | Depth reduction |
|--------------|-----------------|---------------------|-----------------|
| gt-csl.3.5 | 4 | 8 | 0.5 |
| gt-csl.6.1 | 2 | 3 | 0.3 |

## Top-K Set (maximum unblock coverage)

| Bead | Marginal Gain | Unblocks |
|------|--------------|----------|
| gt-csl.3.5 | 4 | 3.6, 4.3, 5.1, 5.3 |
| gt-csl.3.2 | 2 | 3.4, 4.1 |
| gt-csl.6.1 | 2 | 6.2, 6.4 |
| gt-csl.2.2 | 1 | 4.2 |

## Parallel Cut Suggestions

| Bead | Parallel Gain | Enables |
|------|--------------|---------|
| gt-csl.3.5 | 3 | 3.6, 4.3, 5.1, 5.3 |
| gt-csl.3.2 | 1 | 3.4, 4.1 |
| gt-csl.6.1 | 1 | 6.2, 6.4 |

## Verdict

Graph is well-structured. No cycles, no alerts, no missing deps, no duplicates. The bottleneck at gt-csl.3.5 is by design — it's the natural convergence of analysis results before convoy creation. The critical path of depth 8 is reasonable for a 21-task feature with 7 waves.
