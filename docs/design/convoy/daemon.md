# Convoy Manager Flow

> Event-driven convoy completion/feeding and periodic stranded scan

---

## Flow

```mermaid
flowchart TB
    subgraph eventDriven [Event-Driven]
        eventTick["Ticker 5s"] --> poll["store.GetAllEventsSince(lastEventID)"]
        poll --> closed{"Close event?"}
        closed -->|No| eventTick
        closed -->|Yes| check["convoy.CheckConvoysForIssue"]
        check --> advance["Advance high-water mark"]
        advance --> eventTick
    end

    concurrent["Runs concurrently"]
    eventDriven --> concurrent

    subgraph periodicScan [Periodic Scan]
        scanTick["Ticker 30s"] --> stranded["gt convoy stranded --json"]
        stranded --> each["For each convoy"]
        each --> ready{"ready_count > 0?"}
        ready -->|Yes| sling["gt sling issueID rig --no-boot"]
        ready -->|No| close["gt convoy check id"]
        sling --> scanTick
        close --> scanTick
    end

    concurrent --> periodicScan
```

Runs as two goroutines inside `gt daemon`:

- **Event-driven**: Polls beads SDK `GetAllEventsSince` every 5 seconds, detects
  issue close events, and invokes `convoy.CheckConvoysForIssue` to check
  completion and feed the next ready issue. On poll errors it logs and retries
  on the next tick.

- **Periodic scan**: Every 30s, runs `gt convoy stranded --json`. For convoys with
  ready work, dispatches first issue via `gt sling`. For empty convoys, runs
  `gt convoy check` to auto-close. Catches stranded convoys (e.g. after crash)
  that the event poll path missed.

---

## Key Files

| Component | File |
|-----------|------|
| Manager implementation | `internal/daemon/convoy_manager.go` |
| Started by | `internal/daemon/daemon.go` (`Run`) |
