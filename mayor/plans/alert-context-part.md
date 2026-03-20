# Plan: Alert Context Content Part

**Status:** Design phase

## Problem

The alert card artifact currently stores only an `alertId` pointer and fetches live alert state independently via tRPC polling (`useQuery` in the UI component). This creates two sources of truth for alert state within a thread: the agent sees whatever was in the tool result text at invocation time, while the UI card polls the alerts API directly. When a user resolves or mutes an alert, the mutation updates the database, but the agent has no awareness of the change until its next invocation — and even then, it only sees stale text from the original tool call, not structured state.

The result: the agent and the UI can disagree about whether an alert is open, resolved, or muted. The card shows live state; the agent sees historical text. There is no single structured representation of alert state in the thread that both consumers share.

## Solution

Introduce `alert_context` as a first-class content part in the thread model, parallel to Henry's `slack_context` (PR #3273). `alert_context` is a structured representation of alert state that lives in the thread's message content array. It serves as the single source of truth for alert state within a conversation:

- The **agent** reads `alert_context` from thread history on each invocation and always sees current alert state (status, severity, what happened, how to fix).
- The **alert card UI** renders from `alert_context` instead of polling the alerts API — no `useQuery`, no independent data fetching.
- **Alert mutations** (resolve, mute, unmute) update the `alert_context` part in the thread, and the card reflects the change automatically.

This eliminates the divergence between what the agent knows and what the card shows.

## The `alert_context` Content Part

### Shape

```typescript
export const alertContextPartSchema = z.object({
  type: z.literal("alert_context"),
  alertId: z.string().uuid(),
  name: z.string(),
  severity: z.enum(["critical", "error", "warning", "info"]),
  status: z.enum(["open", "resolved", "muted"]),
  whatHappened: z.string(),
  whyItHappened: z.string().optional(),
  howToFix: z.string().optional(),
  triggeredAt: z.string().datetime(),
  resolvedAt: z.string().datetime().optional(),
  mutedAt: z.string().datetime().optional(),
});

export type AlertContextPart = z.infer<typeof alertContextPartSchema>;
```

### Design Decisions

**Why a content part, not an artifact?** Artifacts are UI-only — the agent history loader explicitly excludes them from model input (`toUiMessagePart` returns `undefined` for artifacts). `alert_context` must be visible to the agent, so it belongs in the content part union alongside `message`, `tool_call`, `summary`, etc.

**Why structured fields, not free text?** The agent already receives a text summary from the `trigger_alert` tool result. But text is lossy — you can't reliably extract severity or status from prose. Structured fields let the agent (and UI) read specific values without parsing. The agent can reason about `status: "resolved"` directly; the UI can render a badge from `severity: "critical"` without string matching.

**Why embed state instead of just a pointer?** The current artifact stores only `alertId` and fetches state on render. This requires the UI to make an API call per card, and the agent never sees live state at all. Embedding the state snapshot in the content part means both consumers read from the same place. The tradeoff is that the content part must be updated when alert state changes (see "Alert Mutations" below).

**Why `triggeredAt` as ISO string?** Content parts are stored as JSONB. Dates serialize as strings in JSON. Using ISO 8601 strings keeps the schema self-contained without requiring date parsing at the Zod validation layer.

## Where It Lives

### Registration

1. **Schema definition**: `packages/threads/src/content-parts.ts` — add `alertContextPartSchema` to the `contentPartSchema` union.

2. **Type export**: Same file — export `AlertContextPart` type.

3. **Inngest event schema**: `packages/inngest/src/events/threads.ts` — mirror the schema in the local content part union (required to avoid circular dependency between `@sazabi/threads` and `@sazabi/inngest`).

4. **Thread metadata extractor**: `packages/threads/src/thread-message-metadata.ts` — `alert_context` parts should NOT contribute to thread list previews (same treatment as artifacts and tool calls). No change needed if the extractor already ignores unknown types, but verify.

### Not an Artifact

`alert_context` is NOT added to `ARTIFACT_NAMES` in `packages/artifacts/src/index.ts`. It is a content part, not an artifact. The existing `alertCard` artifact will be deprecated once `alert_context` is fully rolled out (see Migration below).

## Thread History Loader: Hydration

The key behavioral change: the thread history loader hydrates `alert_context` with live state on each agent invocation.

### Current Flow (artifacts excluded)

```
loadThreadMessages() → fetchRecentThreadMessages() → toUiMessage() → toUiMessagePart()
                                                                         ↓
                                                              artifacts → undefined (excluded)
```

### New Flow (alert_context hydrated)

```
loadThreadMessages() → fetchRecentThreadMessages()
                         ↓
                    hydrateAlertContext(rows)  ← NEW: fetch live alert state, patch parts
                         ↓
                    toUiMessage() → toUiMessagePart()
                                       ↓
                              alert_context → text part for model
```

### Hydration Logic

In `services/agent/lib/thread-history.ts`, after fetching message rows but before converting to UI messages:

```typescript
const hydrateAlertContext = async (
  rows: MessageRow[],
): Promise<MessageRow[]> => {
  // 1. Collect all alertIds from alert_context parts across all messages
  const alertIds = new Set<string>();
  for (const row of rows) {
    for (const part of row.content) {
      if (part.type === "alert_context") {
        alertIds.add(part.alertId);
      }
    }
  }

  if (alertIds.size === 0) return rows;

  // 2. Batch-fetch current alert state
  const alerts = await fetchAlertsByIds([...alertIds]);
  const alertMap = new Map(alerts.map((a) => [a.id, a]));

  // 3. Patch alert_context parts with live state
  return rows.map((row) => ({
    ...row,
    content: row.content.map((part) => {
      if (part.type !== "alert_context") return part;
      const live = alertMap.get(part.alertId);
      if (!live) return part; // Alert deleted? Keep stale snapshot.
      return {
        ...part,
        status: live.status,
        severity: live.severity,
        resolvedAt: live.resolvedAt?.toISOString() ?? undefined,
        mutedAt: live.mutedAt?.toISOString() ?? undefined,
      };
    }),
  }));
};
```

**Why hydrate at load time, not write time?** Alert state can change between agent invocations (user resolves alert from the UI, external system sends update). Writing to the thread on every state change would create message churn. Instead, the snapshot in the thread captures the state at creation time, and the loader patches it with live state before the agent sees it. The agent always gets current truth.

**Batch fetch**: A thread might reference multiple alerts (e.g., user asks about several incidents). The hydration collects all `alertId`s and fetches them in a single query, avoiding N+1.

### Model Input Conversion

In `toUiMessagePart`, convert `alert_context` to a structured text block for the model:

```typescript
if (part.type === "alert_context") {
  const lines = [
    `[Alert: ${part.name}]`,
    `Status: ${part.status}`,
    `Severity: ${part.severity}`,
    `Triggered: ${part.triggeredAt}`,
    `What happened: ${part.whatHappened}`,
  ];
  if (part.whyItHappened) lines.push(`Why: ${part.whyItHappened}`);
  if (part.howToFix) lines.push(`How to fix: ${part.howToFix}`);
  if (part.resolvedAt) lines.push(`Resolved: ${part.resolvedAt}`);
  if (part.mutedAt) lines.push(`Muted: ${part.mutedAt}`);
  return { type: "text" as const, text: lines.join("\n") };
}
```

This gives the agent structured context it can reason about without parsing prose.

## Alert Card Artifact: Pure Renderer

### Current Alert Card

The alert card component currently:
1. Receives `alertId` from the artifact data
2. Calls `trpc.alerts.get.useQuery({ alertId })` to fetch live state
3. Polls on an interval for status updates
4. Renders alert details from the query result

### New Alert Card

The alert card becomes a pure renderer of `alert_context`:

1. Receives the full `alert_context` part (not just `alertId`)
2. Renders directly from the part's fields — no `useQuery`, no polling
3. Status, severity, timestamps all come from the content part
4. When the thread updates (via mutation or real-time subscription), the card re-renders automatically

```typescript
// Before: fetches its own data
const AlertCard = ({ alertId }: { alertId: string }) => {
  const { data: alert } = trpc.alerts.get.useQuery({ alertId });
  if (!alert) return <Skeleton />;
  return <AlertCardView alert={alert} />;
};

// After: renders from content part
const AlertCard = ({ context }: { context: AlertContextPart }) => {
  return <AlertCardView alert={context} />;
};
```

**Why remove polling?** The card no longer needs to independently track alert state. When a mutation happens (resolve, mute), the mutation handler updates the `alert_context` part in the thread (see below), and the thread's real-time subscription delivers the update to the card. One source of truth, one update path.

## Alert Mutations: Updating `alert_context`

When a user resolves, mutes, or unmutes an alert, the mutation handler must update both the alerts table AND the `alert_context` part in the thread.

### Current Mutation Flow

```
User clicks "Resolve" → alerts.resolve mutation → UPDATE alerts SET status='resolved'
                                                → postSlackStatusUpdate()
```

### New Mutation Flow

```
User clicks "Resolve" → alerts.resolve mutation → UPDATE alerts SET status='resolved'
                                                → updateAlertContextInThread()  ← NEW
                                                → postSlackStatusUpdate()
```

### `updateAlertContextInThread`

```typescript
const updateAlertContextInThread = async (
  alertId: string,
  updates: Partial<Pick<AlertContextPart, "status" | "resolvedAt" | "mutedAt">>,
) => {
  // 1. Find the message containing alert_context for this alertId
  const message = await findMessageWithAlertContext(alertId);
  if (!message) return; // Alert was created before alert_context existed

  // 2. Patch the alert_context part
  const updatedContent = message.content.map((part) => {
    if (part.type === "alert_context" && part.alertId === alertId) {
      return { ...part, ...updates };
    }
    return part;
  });

  // 3. Update the message content
  await db
    .update(schema.messages)
    .set({ content: updatedContent })
    .where(eq(schema.messages.id, message.id));

  // 4. Emit thread update event (triggers real-time push to UI)
  await inngest.send({
    name: "thread/message.updated",
    data: { threadId: message.threadId, messageId: message.id },
  });
};
```

### Touch Points in `lambdas/api/src/routers/alerts.ts`

Each mutation adds a call to `updateAlertContextInThread`:

| Mutation | Updates |
|----------|---------|
| `resolve` | `{ status: "resolved", resolvedAt: now.toISOString() }` |
| `mute` | `{ status: "muted", mutedAt: now.toISOString() }` |
| `unmute` | `{ status: "open", mutedAt: undefined }` |

## Content Part Converter: Emitting `alert_context`

### `trigger_alert` Tool Changes

The `trigger_alert` tool currently returns `[content, alertCardArtifact]`. It will additionally (or instead) emit an `alert_context` part.

In `packages/tools/src/alerts/tools/trigger-alert.ts`:

```typescript
// Current: returns artifact with alertId pointer
artifact = {
  type: "artifact",
  name: ARTIFACT_NAMES.alertCard,
  data: { alertId },
};

// New: also emit alert_context content part
// The content-part-converter in durable-agent extracts this
// from the tool result and adds it to the message's content array
```

### Converter Changes

In `services/durable-agent/src/workflows/chat/content-part-converter.ts`:

The converter already handles artifact extraction from tool results. `alert_context` can be emitted as a separate content part alongside the artifact (during migration) or as a replacement.

**Option A (migration period):** Tool returns both artifact and `alert_context`. Converter emits both. UI checks for `alert_context` first, falls back to artifact for old messages.

**Option B (post-migration):** Tool returns only `alert_context`. No artifact emitted. Alert card renders from `alert_context` part. Old messages with `alertCard` artifacts continue to work with the legacy fetching card.

Recommend **Option A** for the initial rollout, switching to **Option B** once all active threads have been re-invoked with the new tool.

## Feature Flag

Gate the new behavior behind **`"alert-context-part"`**.

| Component | Flag check | Behavior when off |
|-----------|-----------|-------------------|
| `trigger_alert` tool | Before emitting `alert_context` part | Emit only the `alertCard` artifact (current behavior) |
| Alert card component | Check for `alert_context` in content parts | Fall back to `useQuery` fetching from artifact's `alertId` |
| Alert mutations | Before calling `updateAlertContextInThread` | Skip thread content update (current behavior) |
| Thread history loader | Before hydration pass | Skip hydration (current behavior) |

This allows incremental rollout per organization.

## Migration

### Phase 1: Dual-emit (flag on for early adopters)

- `trigger_alert` emits both `alertCard` artifact and `alert_context` part
- Alert card checks for `alert_context` first, falls back to artifact + polling
- Mutations update `alert_context` when present
- History loader hydrates `alert_context` when present

### Phase 2: Default on

- Flag enabled for all organizations
- New threads always get `alert_context`
- Old threads with only `alertCard` artifacts continue to work (card falls back to polling)

### Phase 3: Deprecate artifact

- Remove `alertCard` from `ARTIFACT_NAMES`
- Remove artifact emission from `trigger_alert`
- Alert card only renders from `alert_context`
- Old messages with `alertCard` artifact render a static "Alert: {alertId}" text (graceful degradation)

No data migration needed — old messages are immutable. The content part union is additive (new types don't break old messages).

## Implementation Checklist

- [ ] `packages/threads/src/content-parts.ts` — add `alertContextPartSchema` to content part union, export `AlertContextPart` type
- [ ] `packages/inngest/src/events/threads.ts` — mirror `alert_context` schema in local content part union
- [ ] `packages/threads/src/thread-message-metadata.ts` — verify `alert_context` is excluded from thread previews (should be by default, but confirm)
- [ ] `services/agent/lib/thread-history.ts` — add `hydrateAlertContext` step in `loadThreadMessages`, gated on feature flag
- [ ] `services/agent/lib/thread-history.ts` — add `alert_context` case in `toUiMessagePart` for model input conversion
- [ ] `packages/tools/src/alerts/tools/trigger-alert.ts` — emit `alert_context` part alongside `alertCard` artifact, gated on feature flag
- [ ] `services/durable-agent/src/workflows/chat/content-part-converter.ts` — handle `alert_context` extraction from tool results
- [ ] `lambdas/api/src/routers/alerts.ts` — add `updateAlertContextInThread` call to `resolve`, `mute`, `unmute` mutations, gated on feature flag
- [ ] Alert card UI component — accept `AlertContextPart` props, render from content part instead of `useQuery`, with fallback to legacy artifact behavior
- [ ] Create `alert-context-part` feature flag in PostHog

## Open Questions

1. **Should `alert_context` include monitor/check metadata?** The current shape focuses on the alert itself. If the agent needs to understand the underlying monitor (check type, threshold, region), that could be a separate `monitor_context` part or additional fields on `alert_context`. Defer unless agents frequently need monitor details to help users.

2. **Real-time push for hydrated state?** The plan has mutations updating `alert_context` in the thread and emitting an update event. But if an alert is resolved outside the thread (e.g., auto-resolve from monitoring), the thread's `alert_context` won't update until the next agent invocation hydrates it. Is this acceptable, or should we add an Inngest subscriber on `alert.status.changed` that proactively updates all threads referencing that alert? Likely overkill for v1 — the hydration-on-load pattern handles it for the agent, and the UI fallback to polling covers the gap during migration.

3. **Multiple alerts per thread?** A thread can discuss multiple alerts. Each `trigger_alert` call emits its own `alert_context` part in a separate assistant message. The hydration logic already handles this (collects all `alertId`s across all messages). The alert card UI needs to identify which `alert_context` in the thread corresponds to which card — the `alertId` field provides this mapping. No special handling needed.

4. **Thread compaction and summaries?** When a thread is compacted (messages replaced with a summary), `alert_context` parts in compacted messages are lost. The summary text should include a mention of the alert (the summarizer sees the text conversion of `alert_context`). But the structured part is gone. If the agent needs structured alert state after compaction, the tool can be re-invoked. This matches the existing pattern — tool call results are also lost on compaction, and the agent re-executes tools when needed.
