<!-- ox-hash: cba0cb90960d ver: 0.4.0 -->
Load SageOx team context for this AI coworker session.

Use when:
- Starting a new coding session in a repo with shared team context
- After context compaction or clear operations
- When you need team conventions, norms, or architectural decisions
- Before making changes to understand SageOx team patterns

Keywords: prime, session start, guidance, team context, conventions, init session

## Common Issues

### ox not found
**Symptom:** `command not found: ox`
**Solution:** Install ox CLI: `brew install ghostlayer/tap/ox` or see installation docs

### No guidance loaded
**Symptom:** Prime runs but returns empty guidance
**Solution:** Run `ox init` first to initialize SageOx in this repository

### Stale guidance
**Symptom:** Guidance doesn't reflect recent changes
**Solution:** Run `ox agent prime --refresh` to reload from source

$ox agent prime
