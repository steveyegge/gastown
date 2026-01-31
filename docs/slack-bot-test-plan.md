# Slack Bot Integration Test Plan

## Prerequisites Checklist

- [ ] gtmobile running on `http://localhost:8443`
- [ ] gtslack built: `go build -o gtslack ./cmd/gtslack`
- [ ] Slack app configured per setup guide
- [ ] Bot token (`xoxb-...`) obtained
- [ ] App token (`xapp-...`) obtained
- [ ] Notification channel ID obtained
- [ ] Bot invited to test channel

## Test Scenarios

### T1: Bot Startup and Connection

**Steps:**
1. Start gtmobile: `./gtmobile --town /path/to/town --port 8443`
2. Start gtslack with tokens
3. Observe startup logs

**Expected:**
```
Starting Gas Town Slack bot
RPC endpoint: http://localhost:8443
Notifications channel: C...
Starting SSE listener: http://localhost:8443/events/decisions
Slack: Connecting to Socket Mode...
Slack: Connected to Socket Mode
SSE: Connected to decision events stream
```

**Pass criteria:** All three connections successful (Slack, RPC health, SSE)

---

### T2: List Decisions - Empty State

**Precondition:** No pending decisions in beads

**Steps:**
1. In Slack, type `/decisions`

**Expected:**
- Ephemeral message appears
- Shows "No pending decisions!" with friendly message

**Pass criteria:** Empty state displays correctly

---

### T3: List Decisions - With Pending

**Precondition:** Create test decisions:
```bash
gt decision request --prompt "Test decision 1" --option "Yes" --option "No" --urgency high
gt decision request --prompt "Test decision 2" --option "A" --option "B" --urgency medium
```

**Steps:**
1. In Slack, type `/decisions`

**Expected:**
- Summary shows "2 Pending Decisions" with urgency counts
- Each decision shows:
  - Urgency indicator (red/yellow/green circle)
  - Decision ID
  - Question (truncated if long)
  - "View" button

**Pass criteria:** All decisions listed with correct formatting

---

### T4: View Decision Details

**Steps:**
1. Run `/decisions`
2. Click "View" button on a decision

**Expected:**
- Ephemeral message with full decision details:
  - Decision ID
  - Full question
  - Context (if any)
  - Options as buttons

**Pass criteria:** Details display correctly with all options visible

---

### T5: Resolve Decision via Modal

**Steps:**
1. View a decision (T4)
2. Click an option button
3. In modal, enter rationale: "Test resolution"
4. Click "Resolve"

**Expected:**
- Modal closes
- Confirmation message appears:
  - "Decision Resolved Successfully!"
  - Shows decision ID
  - Shows chosen option label
  - Shows rationale
  - Context hint about notifications

**Pass criteria:** Decision resolved, confirmation shown

---

### T6: Verify Resolution in Beads

**After T5:**
```bash
bd show <decision-id>
```

**Expected:**
- Decision shows as resolved
- Rationale includes user attribution
- Close reason shows choice

**Pass criteria:** Beads reflects resolution

---

### T7: Real-time SSE Notification

**Precondition:** gtslack running with notification channel configured

**Steps:**
1. Create decision via CLI:
   ```bash
   gt decision request --prompt "SSE test" --option "Yes" --option "No"
   ```
2. Observe notification channel

**Expected:**
- Message appears in channel within seconds:
  - "New Decision Required"
  - Decision ID
  - Question text
  - "View & Resolve" button

**Pass criteria:** Notification appears automatically

---

### T8: SSE Reconnection

**Steps:**
1. With gtslack running, stop gtmobile: `pkill gtmobile`
2. Observe gtslack logs
3. After 5 seconds, restart gtmobile
4. Create a new decision

**Expected:**
- gtslack logs show: "Connection error... reconnecting in Xs"
- After gtmobile restarts, logs show: "Connected to decision events stream"
- New decisions still trigger notifications

**Pass criteria:** Automatic reconnection works

---

### T9: Error Handling - Decision Already Resolved

**Steps:**
1. List decisions, click View on one
2. In another terminal, resolve the same decision via CLI
3. Back in Slack, try to resolve via modal

**Expected:**
- Error message with troubleshooting hint
- "This decision may have already been resolved by someone else"

**Pass criteria:** Graceful error handling

---

### T10: Error Handling - RPC Unavailable

**Steps:**
1. Stop gtmobile
2. Run `/decisions` in Slack

**Expected:**
- Error message appears
- Includes hint: "Gas Town server may be temporarily unavailable"

**Pass criteria:** Helpful error message

---

## Test Results Template

| Test | Status | Notes |
|------|--------|-------|
| T1: Startup | | |
| T2: Empty list | | |
| T3: List pending | | |
| T4: View details | | |
| T5: Resolve modal | | |
| T6: Verify beads | | |
| T7: SSE notification | | |
| T8: SSE reconnect | | |
| T9: Already resolved | | |
| T10: RPC unavailable | | |

## Quick Verification Commands

```bash
# Check gtmobile health
curl -s http://localhost:8443/health

# List pending decisions via RPC
curl -s -X POST http://localhost:8443/gastown.v1.DecisionService/ListPending \
  -H "Content-Type: application/json" -d "{}"

# Check SSE stream
curl -sN http://localhost:8443/events/decisions

# Check metrics
curl -s http://localhost:8443/metrics

# Create test decision
gt decision request --prompt "Integration test" \
  --option "Option A: First choice" \
  --option "Option B: Second choice" \
  --urgency medium
```
