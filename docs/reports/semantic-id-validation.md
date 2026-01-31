# Semantic ID Validation Report (v0.2 Format)

**Generated**: 2026-01-29
**Format**: `<prefix>-<type>-<slug><suffix>`
**Spec**: docs/design/semantic-id-spec.md

## Summary Statistics
- **Total issues analyzed**: 2,622
- **Format**: `prefix-type-slug+suffix` (e.g., `gt-epc-semantic_ids7x9k`)

### Collision Analysis
- **Slug collisions (before suffix)**: 1,202 (45.8%)
  - Patrol/Molecule (ephemeral): 1,014
  - Work beads (persistent): 188
- **Full ID collisions (with suffix)**: 0 (effectively 0%)
  - Random suffix provides 1.6M+ unique combinations

## Type Distribution
| Type Code | Count | Percentage |
|-----------|-------|------------|
| `tsk` | 1,839 | 70.1% |
| `unk` | 433 | 16.5% |
| `bug` | 157 | 6.0% |
| `ftr` | 75 | 2.9% |
| `epc` | 68 | 2.6% |
| `wsp` | 17 | 0.6% |
| `cnv` | 12 | 0.5% |
| `mol` | 10 | 0.4% |
| `agt` | 9 | 0.3% |
| `rol` | 2 | 0.1% |

## Slug Length Distribution
- Average slug length: 28.0 chars
- Average total ID length: 39.0 chars

-  1-10 chars:    5 (  0.2%) 
- 11-20 chars:  208 (  7.9%) ███
- 21-30 chars: 1437 ( 54.8%) ███████████████████████████
- 31-40 chars:  972 ( 37.1%) ██████████████████

## Top 15 Colliding Slugs (before suffix)
| Slug (prefix-type-slug) | Count | Category |
|-------------------------|-------|----------|
| `gt-tsk-digest_mol_deacon_patrol` | 930 | Patrol |
| `gt-tsk-digest_mol_refinery_patrol` | 27 | Patrol |
| `hq-wsp-mol_witness_patrol` | 15 | Patrol |
| `hq-tsk-end_of_cycle_inbox_hygiene` | 8 | Work |
| `hq-tsk-process_witness_mail` | 8 | Work |
| `hq-tsk-check_if_active_swarm_is_complete` | 8 | Work |
| `hq-tsk-ensure_refinery_is_alive` | 8 | Work |
| `hq-tsk-check_own_context_limit` | 8 | Work |
| `hq-tsk-inspect_all_active_polecats` | 8 | Work |
| `hq-tsk-process_pending_cleanup_wisps` | 8 | Patrol |
| `hq-tsk-loop_or_exit_for_respawn` | 8 | Work |
| `hq-tsk-ping_deacon_for_health_check` | 8 | Work |
| `hq-tsk-check_timer_gates_for_expiration` | 8 | Work |
| `hq-tsk-respawn_orphaned_polecats` | 8 | Work |
| `hq-tsk-digest_mol_witness_patrol` | 7 | Patrol |

## Work Beads Analysis (bugs, tasks, epics, features)
- **Total work beads**: 2,139
- **Slug collision rate (before suffix)**: 53.11%
- **With random suffix**: ~0% (suffix resolves all collisions)

## Sample Generated IDs
| Original ID | Type | Semantic ID | Slug Len |
|-------------|------|-------------|----------|
| `bd-0h7` | unk | `bd-unk-decision_what_should_dolt_fix_cre...` | 34 |
| `bd-0u7` | unk | `bd-unk-decision_two_beads_ready_in_paral...` | 36 |
| `bd-18u` | unk | `bd-unk-decision_beads_keep_disappearing_...` | 39 |
| `bd-25k` | unk | `bd-unk-decision_systemd_service_installe...` | 39 |
| `bd-279` | unk | `bd-unk-merge_rebase_upstream5l5m` | 21 |
| `bd-2it` | unk | `bd-unk-decision_how_to_fix_dolt_servertq...` | 31 |
| `bd-2us` | unk | `bd-unk-decision_how_to_handle_the_15_dol...` | 39 |
| `bd-2zl` | unk | `bd-unk-decision_how_should_we_fix_the_do...` | 35 |
| `bd-3iz` | unk | `bd-unk-decision_daemon_test_passed_aws_s...` | 36 |
| `bd-3n7` | unk | `bd-unk-decision_config_fixed_epic_recrea...` | 36 |
| `bd-3un` | unk | `bd-unk-decision_what_should_dolt_fix_wor...` | 37 |
| `bd-3yn` | unk | `bd-unk-merge_rebase_upstream444f` | 21 |
| `bd-4h2` | unk | `bd-unk-merge_hq_81z1z8d` | 12 |
| `bd-4op` | unk | `bd-unk-decision_config_migrated_to_centr...` | 38 |
| `bd-5js` | unk | `bd-unk-decision_research_phase_complete_...` | 37 |
| `gt-00qjk` | bug | `gt-bug-bug_convoys_don_t_auto_close_when...` | 37 |
| `gt-01566` | unk | `gt-unk-session_ended_gt_gastown_crew_joe...` | 33 |
| `gt-01jpg` | bug | `gt-bug-autonomous_agents_idle_at_welcome...` | 33 |
| `gt-02431` | ftr | `gt-ftr-implement_queue_name_address_type...` | 37 |
| `gt-031qp` | unk | `gt-unk-session_ended_gt_gastown_crew_gus...` | 33 |

## Validation Results

### Acceptance Criteria

- ✅ **Generated IDs are readable and meaningful**
  - Type code + semantic slug provides clear meaning
- ✅ **Type visible in ID**
  - Type codes: agt, bug, cnv, epc, ftr, mol, rol, tsk, unk, wsp
- ✅ **Collision-proof with suffix**
  - Full ID collisions: 0
- ❌ **Slug collisions acceptable (<5% for work)**
  - Work bead slug collision rate: 53.11%
- ✅ **Length distribution reasonable**
  - Average slug length: 28.0 chars

### Recommendation
**REVIEW NEEDED** - Some criteria not met.

### Implementation Notes
- Format: `<prefix>-<type>-<slug><suffix>`
- Example: `gt-epc-semantic_ids7x9k`
- Type codes make filtering easy: `bd list | grep 'gt-bug-'`
- Random suffix (4 chars) guarantees uniqueness
- Patrol/molecule beads can optionally keep random IDs
