# Medici Dispatch: Rally Tavern Integration

**Convoy:** hq-cv-mjqo1
**Dispatch mode:** Self-execution (polecats cannot sling; all lenses executed by furiosa)

| Lens | Bead ID | Assignee | Output Path |
|------|---------|----------|-------------|
| domain | gt-0ri | furiosa (self) | `.medici/gt-4kw/domain.md` |
| agent-ux | gt-raq | furiosa (self) | `.medici/gt-4kw/agent-ux.md` |
| topology | gt-umg | furiosa (self) | `.medici/gt-4kw/topology.md` |
| incentives | gt-alh | furiosa (self) | `.medici/gt-4kw/incentives.md` |
| constraints | gt-xnf | furiosa (self) | `.medici/gt-4kw/constraints.md` |
| adversary | gt-4od | furiosa (self) | `.medici/gt-4kw/adversary.md` |

**Note:** `gt sling` returned "polecats cannot sling". All lens analyses will be
executed sequentially by the assigned polecat (furiosa) rather than dispatched
in parallel to separate workers. This reduces parallelism but maintains the
multi-lens structure of the Medici process.
