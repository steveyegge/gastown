# Scalability Analysis

## Summary

Faultline is a single-binary Go service backed by Dolt (a MySQL-compatible, git-versioned database) on port 3307. The explicit design ceiling is **100 events/second sustained** (ADR-5), targeting small-to-mid teams where the Gas Town agentic loop adds value. This analysis examines every subsystem through the lens of data size, request rate, resource usage, and long-term growth to identify bottlenecks, hard limits, and optimization opportunities.

The architecture is fundamentally sound for its stated target. The main risks are not in the hot path (ingest is clean and fast) but in **write amplification** (3-7 DB round trips per event), **Dolt-specific commit overhead** (growing working sets slow `dolt_add`/`dolt_commit`), **dashboard query patterns** that degrade with data size (N+1 queries, missing indexes, correlated subqueries), and **unbounded growth** in several in-memory structures and the Dolt chunk store over months of operation.

## Analysis

### Key Considerations

- **Target: 100 ev/s sustained.** This is a deliberate, documented constraint (ADR-5). The architecture is not designed for fleet-scale (1000+ ev/s). At the target, the system processes ~8.6M events/day.
- **Dolt is the bottleneck, not Go.** The Go process can handle thousands of goroutines and HTTP connections. Dolt's write throughput, commit overhead, and diff-table query cost are the binding constraints.
- **Single-process, single-server.** No horizontal scaling path exists today. Faultline is one Go binary + one Dolt server. This is a feature (simplicity) and a constraint (vertical scaling only).
- **Write amplification is the core concern.** Every inbound event generates 3-7 sequential DB writes (issue_groups upsert + SELECT, ft_events INSERT, projects heartbeat UPDATE, optional releases upsert, optional lifecycle INSERT, optional bead filing). At 100 ev/s, this translates to 300-700 DB operations/second on the ingest path alone, before accounting for background workers and dashboard queries.

### Scale Dimensions

#### Data Size

| Metric | At 10 ev/s | At 100 ev/s | At 100 ev/s + 90 days |
|--------|-----------|------------|----------------------|
| Events/day | 864K | 8.64M | 777M total |
| Raw JSON storage/day | ~860 MB | ~8.6 GB | ~777 GB total |
| Dolt commits/day | 1,440 | 1,440 | 129,600 total |
| Issue groups (est.) | ~100-1K | ~1K-10K | ~10K-100K |
| Lifecycle events | ~5K/day | ~50K/day | ~4.5M total |

The `ft_events` table with `raw_json JSON NOT NULL` is the dominant storage consumer. Average event payloads are 1-50 KB. Dolt's chunk store is **append-only** -- even after the 90-day retention worker deletes rows, old commit history preserves the data. Without periodic `dolt gc`, the chunk store grows roughly as: `event_size x events/s x 60s x 1440_commits/day x days_retained`.

#### Request Rate

The ingest path is the hot loop:

```
HTTP POST /api/{project_id}/envelope/
  -> rate limit check (in-memory, O(1))
  -> auth check (in-memory map, O(1))
  -> parse envelope (CPU-bound, memory allocation)
  -> per item:
     -> fingerprint (SHA-256, CPU-bound)
     -> UpsertIssueGroup (DB write + DB read)
     -> InsertEvent (DB write)
     -> RecordHeartbeat (DB write)
     -> optional: UpsertRelease, lifecycle, bead filing
```

At 100 ev/s with 3-5 DB round trips each at ~1-2ms per query, the ingest path needs ~300-500ms of cumulative DB time per second, consuming 3-5 of the 25 available connection pool slots continuously. This leaves ~20 connections for background workers (9 goroutines) and dashboard queries.

#### User/Connection Count

- **Connection pool**: 25 max open, 10 idle (configured in `dolt.go`).
- **Dashboard viewers**: Each SSE `/dashboard/live/events` endpoint holds an open connection + goroutine. 10 concurrent viewers = 10 persistent connections from the pool.
- **Background workers**: 9 goroutines, each acquiring connections periodically. The `dbmon` worker acquires one every 1 second regardless of configuration.
- **Effective ingest headroom**: At 100 ev/s with 10 dashboard viewers and 9 background workers, only ~6 connections remain for burst ingest traffic.

### Resource Usage

#### Memory

| Component | Growth Pattern | Bound |
|-----------|---------------|-------|
| Auth map (`map[string]int64`) | O(projects) | Bounded, negligible |
| Rate limiter map | O(projects) | Bounded, negligible |
| Fingerprint rule cache | O(projects x rules) | Bounded, flushed on miss |
| `gastown.Tracker` event windows | O(unique issue groups) | **Unbounded** -- maps never GC'd |
| `gastown.Tracker` notified/failedFix | O(unique issue groups) | **Unbounded** -- only shrinks on Reset() |
| `dashboard.loginAttempts` | O(IPs x 15min window) | Bounded by time, **but not mutex-protected (data race)** |
| Envelope parsing | Up to 200 MB per request | 25 concurrent = 5 GB worst case |

The `gastown.Tracker` is the primary memory concern for long-running instances. Every unique issue group ID that has ever triggered an event is stored in `map[string][]time.Time` and `map[string]bool` structures with no LRU eviction or size cap.

The 200 MiB max envelope size is very generous (Sentry defaults to 20 MB). At 25 concurrent connections with adversarial payloads, memory pressure reaches 5 GB. Consider reducing to 20 MiB.

#### CPU

- **Fingerprinting**: SHA-256 per event -- fast, O(event_size). Not a concern.
- **Regex matching**: `regexp.MatchString` is called per event per fingerprint rule. This **recompiles the regex on every call** -- no precompiled cache. With 10 rules per project, this is 1000 regex compilations/second at 100 ev/s. Precompiling to `*regexp.Regexp` would reduce CPU by ~10x for rule matching.
- **Gzip decompression**: `gzip.NewReader` allocates per request. At high concurrency, GC pressure increases.
- **JSON marshal/unmarshal**: Heavy allocation pressure on the ingest path. Go's `encoding/json` is not the fastest; `json.RawMessage` is used where possible (good).

#### Disk

- **Raw event storage**: 8.6 GB/day at 100 ev/s (before Dolt versioning overhead).
- **Dolt versioning overhead**: Each commit stores a full Prolly-tree delta. With 1,440 commits/day, the chunk store grows significantly faster than the logical data size. Estimate 2-3x overhead for the commit graph.
- **Retention**: 90-day default. At 100 ev/s, this means ~777 GB of logical event data plus Dolt overhead. **Total disk: estimate 1.5-2.5 TB after 90 days at design ceiling.**
- **Without `dolt gc`**: Disk never shrinks. Deleted rows persist in old commits. This is the single largest long-term scalability concern.

#### Network

- Relay poller: outbound HTTP every 30s (fixed, low impact)
- DB monitoring: outbound TCP per monitored database per check interval
- Uptime monitoring: outbound HTTP per project URL every 60s
- Self-monitoring: outbound HTTP to localhost on every `slog.Error()` call
- Gas Town subprocess: `bd new` / `gt sling` are OS process spawns with potential network calls

### Bottlenecks: What Limits Growth?

#### 1. Write Amplification (Primary Bottleneck)

Every event triggers 3-7 sequential DB operations. There is no write batching, no pipelining, and no coalescing of operations across events. Each operation acquires a connection, executes, and releases it.

**Impact**: At 100 ev/s, Dolt must sustain 300-700 ops/sec on the ingest path alone. Load testing showed 1500 ev/s burst at c=50, but sustained throughput depends on working set size and concurrent reads.

**Mitigation options**:
- **Batch inserts**: Accumulate events in memory for 100ms-1s, then batch-insert. Trades latency for throughput.
- **Debounce heartbeat**: `RecordHeartbeat` fires on every envelope. Debounce to once per project per 10s (10x reduction in heartbeat writes).
- **Pipeline DB operations**: Use a single connection per event with chained queries instead of acquiring/releasing from pool for each.

#### 2. Dolt Commit Overhead (Secondary Bottleneck)

`CALL dolt_add('-A')` + `CALL dolt_commit(...)` runs every 60 seconds. This operation:
- Stages all modified pages across all tables
- Hashes and packs chunks into the Prolly-tree
- Holds internal write locks during commit

As the database grows, commit time increases. At 100 ev/s, each commit covers ~6,000 new events plus heartbeat updates, lifecycle events, and issue group updates. With months of accumulated data, the Prolly-tree becomes deep, and `dolt_add` must scan more pages.

**Impact**: Commit takes 1-5 seconds at moderate data sizes. If commit time exceeds 60s, commits pile up. During commit, write locks may slow concurrent ingest.

**Mitigation options**:
- Monitor commit latency as a health metric
- Increase commit interval dynamically based on working set size
- Run `dolt gc` on a weekly schedule to compact the chunk store

#### 3. Dashboard Query Performance (Tertiary Bottleneck)

Several dashboard queries degrade with data size:

| Query | Issue | Impact at Scale |
|-------|-------|-----------------|
| `GetProjectStats` | 8 sequential COUNT/MAX queries per project | N projects x 8 queries = 80 round trips for 10 projects |
| `HourlyEventCounts` | Table scan with computed columns, no `(project_id, timestamp)` index | Scans millions of rows for 24h window |
| `ListIssueGroups` with env filter | Correlated subquery scans all events | Full `ft_events` scan per page load |
| `LatestLifecycleByGroups` | MAX-join on append-only table | Degrades as lifecycle table grows |
| `DoltLogForIssue` | UNION on `dolt_diff_*` tables | Slows with commit count (43,200/month) |
| Time-travel queries | `AS OF` scans commit history | Linear in commit count |

**Impact**: Dashboard becomes sluggish within weeks at high event volume. Time-travel queries become impractical within months.

**Mitigation options**:
- Add `(project_id, timestamp)` index on `ft_events`
- Pre-aggregate stats in a `project_stats` summary table updated by background worker
- Denormalize `environment` and `release` onto `issue_groups`
- Cap time-travel query depth or add commit pruning

#### 4. Unbounded Goroutine Spawns (Event Storm Risk)

`bridge.OnEvent` spawns `go b.fileBead(...)` with `context.Background()` for every alert rule match. Each fileBead call exec's `bd new` (an OS process). During a deploy-gone-wrong with dozens of new fingerprints, this can spawn dozens of concurrent subprocesses with no bound.

**Impact**: CPU/memory spike, potential OOM or file descriptor exhaustion.

**Mitigation**: Use a bounded worker pool (e.g., `chan struct{}` semaphore with capacity 5-10).

### Algorithmic Complexity

| Operation | Time | Space |
|-----------|------|-------|
| Fingerprint (SHA-256) | O(event_size) | O(1) |
| Auth lookup | O(1) amortized | O(projects) |
| Rate limit check | O(1) | O(projects) |
| UpsertIssueGroup | O(log N) B-tree | O(1) |
| InsertEvent | O(log N) B-tree | O(event_size) |
| GetProjectStats | O(N) per COUNT | O(1) |
| HourlyEventCounts | O(events_in_window) | O(hours) |
| dolt_add | O(dirty_pages) | O(1) |
| dolt_commit | O(dirty_pages) | O(dirty_pages) |
| Time-travel query | O(commits_in_range) | O(result_set) |

### Caching Opportunities

| What | Current | Recommendation | Impact |
|------|---------|----------------|--------|
| Issue group status (ignore check) | DB query per event | In-memory cache with 60s TTL | -100 queries/s at design target |
| Project stats | Computed per page load | Background-refreshed summary table | Dashboard response time 10x improvement |
| Fingerprint rules | Cached per project (good) | Precompile regex patterns | 10x CPU reduction for rule matching |
| Auth session | DB query per request | In-memory LRU with 5min TTL | -N queries per dashboard page view |
| Heartbeat | Write per envelope | Debounce per project per 10s | -90% heartbeat writes |
| Issue group details | DB query in ignore/regression check | LRU cache with event-based invalidation | -100 queries/s at target |

### Degradation Modes: What Happens at Limits?

| Scenario | Behavior | Recovery |
|----------|----------|----------|
| **>100 ev/s per project** | HTTP 429 (rate limiter). Client SDKs back off. | Automatic. SDKs retry with exponential backoff. |
| **Connection pool exhausted** | Ingest requests queue on pool.Get(). Latency spikes. SDKs may timeout. | Reduce concurrent dashboard connections. Increase pool size. |
| **Dolt commit takes >60s** | Next commit tick fires while current is running. No overlap protection (context cancel + new tick). Potential commit queue. | Increase commit interval. Run `dolt gc`. |
| **Disk full** | Dolt writes fail. Events are lost (no write-ahead buffer). | Alert on disk usage. Run `dolt gc`. Reduce retention window. |
| **Event storm (many new fingerprints)** | Unbounded `fileBead` goroutines. Many `bd new` subprocesses. CPU/memory spike. | Add goroutine pool bound. Add bead creation rate limit per project. |
| **Long-running (months)** | `dolt_diff_*` queries slow. Dashboard stats queries slow. Chunk store grows. | Periodic `dolt gc`. Index tuning. Pre-aggregation. |
| **Relay backlog** | Fixed catch-up rate of 200 envelopes/min. Multi-hour outage = hours of catch-up. | Increase poll batch size. Parallel relay processing. |

### Options Explored

#### Option 1: Optimize Within Current Architecture (Recommended for Now)
- **Description**: Fix write amplification, add caching, add missing indexes, debounce heartbeat, bound goroutines, precompile regex. No architectural changes.
- **Pros**: Low risk, incremental, no new dependencies. Extends headroom to ~500 ev/s burst.
- **Cons**: Still single-process, still Dolt-bound. Vertical scaling only.
- **Effort**: Medium (2-3 weeks of focused work)

#### Option 2: Write-Ahead Buffer + Async Processing
- **Description**: Add an in-memory or disk-backed write-ahead buffer between ingest and Dolt. Batch DB writes. Decouple HTTP response from DB persistence.
- **Pros**: Absorbs burst traffic. Reduces write amplification by 5-10x. Enables event coalescing.
- **Cons**: Adds complexity. Events are "in flight" during buffer window (not yet queryable). Potential data loss on crash without disk WAL.
- **Effort**: High (4-6 weeks)

#### Option 3: Read Replicas + Query Separation
- **Description**: Separate read and write paths. Dashboard queries go to a read replica. Ingest writes to primary. Dolt supports replication.
- **Pros**: Eliminates read/write contention on connection pool. Dashboard performance independent of ingest load.
- **Cons**: Adds operational complexity (second Dolt instance). Read replica lag. More infrastructure (contradicts "minimal infrastructure" goal).
- **Effort**: High (3-4 weeks + ongoing ops)

#### Option 4: Replace Dolt for Hot Path
- **Description**: Use SQLite or PostgreSQL for hot event storage, Dolt for versioned archives only.
- **Pros**: Dramatic write performance improvement. Standard DB tooling.
- **Cons**: Loses Dolt's core value proposition (time-travel on live data). Major architectural change. Two databases to manage.
- **Effort**: Very High (6-8 weeks)

### Recommendation

**Option 1 (optimize within current architecture)** is the right approach for the stated 100 ev/s target. The specific optimizations, in priority order:

1. **Debounce `RecordHeartbeat`** -- once per project per 10s instead of per-envelope (immediate 90% write reduction)
2. **Add `(project_id, timestamp)` index on `ft_events`** -- fixes `HourlyEventCounts` table scan
3. **Precompile fingerprint rule regexes** -- cache `*regexp.Regexp` instead of recompiling per call
4. **Cache issue group status in-memory** -- eliminates ignore/regression SELECT per event
5. **Bound `fileBead` goroutines** -- semaphore with capacity 5-10
6. **Pre-aggregate project stats** -- background worker refreshes summary table every 60s
7. **Schedule weekly `dolt gc`** -- prevent chunk store bloat
8. **Fix `loginAttempts` data race** -- add mutex
9. **Denormalize environment onto `issue_groups`** -- eliminate correlated subquery in list filters
10. **Reduce max envelope size** from 200 MiB to 20 MiB

Option 2 (write-ahead buffer) should be revisited if sustained load exceeds 200 ev/s or if Dolt commit latency becomes problematic.

## Constraints Identified

- **Dolt commit granularity**: All-or-nothing `dolt_add('-A')`. Cannot commit events separately from config changes. This is a Dolt limitation.
- **Dolt chunk store is append-only**: Disk grows monotonically without `dolt gc`. This must be operationalized.
- **Single-process architecture**: No horizontal scaling without a fundamental redesign. Acceptable for target audience.
- **25-connection pool**: Shared between ingest, dashboard, background workers, and SSE viewers. Hard ceiling on concurrency.
- **Relay catch-up rate**: Fixed at 200 envelopes/min. Extended outages cause slow recovery.
- **No write-ahead buffer**: Events that fail to write to Dolt are lost. No retry, no dead letter queue.

## Open Questions

1. **What is the expected concurrent dashboard viewer count?** Each SSE connection consumes a persistent connection from the shared pool. 10+ viewers could starve ingest.
2. **Is `dolt gc` safe to run against a live server?** Need to verify Dolt's online GC capabilities before scheduling it.
3. **What is the actual Dolt commit latency at 90 days of data?** Load testing showed burst performance but not sustained performance with large working sets.
4. **Should the relay batch size be configurable?** Fixed at 100 envelopes limits catch-up rate.
5. **What is the expected issue group cardinality?** If thousands of unique fingerprints accumulate, the `gastown.Tracker` memory growth becomes significant.

## Integration Points

- **Technical Feasibility (fl-leg-4vp4u)**: The scalability constraints inform what is technically feasible at the design ceiling. Dolt commit overhead and write amplification are the binding feasibility constraints.
- **Integration Analysis (fl-leg-q67x4)**: The Gas Town bridge (`fileBead` goroutines, bead polling) is a scalability-sensitive integration point. Bead storms map directly to goroutine storms.
- **Scope Analysis (fl-leg-anaxs)**: The 100 ev/s ceiling should be stated as an explicit scope boundary. Features like real-time dashboards with many concurrent viewers may need to be phased based on connection pool constraints.
- **Ambiguity Analysis (fl-leg-uzmke)**: "Sentry SDK compatible" at scale implies session replay, profiling, and attachment handling -- all of which are far more resource-intensive than error events. The scalability analysis assumes error events only.
