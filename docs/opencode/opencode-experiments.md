# Opencode Experimentation Checklist

> Hands-on experiments to validate Opencode orchestration assumptions
> 
> **Purpose**: Answer open questions before full implementation
> **Related**: [opencode-orchestration.md](opencode-orchestration.md), [opencode-implementation-guide.md](opencode-implementation-guide.md)
> **Status**: Planning
> **Updated**: 2026-01-15

## Overview

This document tracks experiments needed to validate the Opencode orchestration plan. Each experiment has clear objectives, test procedures, and success criteria.

## Experiment Categories

- 🟢 **Basic**: Core functionality, required for MVP
- 🟡 **Advanced**: Nice-to-have, affects design choices
- 🔴 **Blocking**: Must resolve before implementation

## Basic Experiments (Required for MVP)

### EXP-001: Multi-Session Spawn 🟢

**Objective**: Verify Opencode can run multiple concurrent sessions on one host.

**Assumptions to Test**:
- Opencode allows multiple processes simultaneously
- Each session remains isolated
- No port conflicts or shared state issues

**Test Procedure**:
```bash
# Terminal 1
cd /tmp/exp-001-session-1
opencode
# Note: session ID, PID, resource usage

# Terminal 2
cd /tmp/exp-001-session-2
opencode
# Note: session ID, PID, resource usage

# Terminal 3
cd /tmp/exp-001-session-3
opencode
# Note: session ID, PID, resource usage

# Verify all running
ps aux | grep opencode
```

**Success Criteria**:
- [ ] All 3 sessions start successfully
- [ ] Each has unique session ID
- [ ] No port conflicts reported
- [ ] Sessions don't interfere with each other
- [ ] Can assign different work to each

**Data to Collect**:
- Memory usage per session (baseline)
- CPU usage per session (idle)
- Startup time per session
- Any error messages

**Expected Outcome**: 3-5 concurrent sessions work fine

**Risk if Failed**: Need to implement session pooling/rotation

**Status**: ⬜ Not Started

---

### EXP-002: Plugin Installation 🟢

**Objective**: Verify the Gastown plugin installs and loads correctly in Opencode.

**Assumptions to Test**:
- Plugin can be installed via file copy
- Opencode loads plugins from `.opencode/plugin/`
- Plugin events fire reliably

**Test Procedure**:
```bash
# Create test workspace
cd /tmp/exp-002-plugin-test
mkdir -p .opencode/plugin

# Copy Gastown plugin
cp /path/to/gastown/internal/opencode/plugin/gastown.js .opencode/plugin/

# Start Opencode with verbose logging
opencode --verbose
# or check if there's a debug/plugin-test mode

# Check if plugin loaded
# (look for plugin initialization messages)
```

**Success Criteria**:
- [ ] Plugin file detected by Opencode
- [ ] Plugin loads without errors
- [ ] `session.created` event fires
- [ ] `gt prime` executes automatically
- [ ] No permission or path issues

**Data to Collect**:
- Plugin load time
- Event firing order
- Any console warnings/errors
- Whether plugin persists across restarts

**Expected Outcome**: Plugin installs and works automatically

**Risk if Failed**: Need manual setup steps for users

**Status**: ⬜ Not Started

---

### EXP-003: Work Assignment via Mailbox 🟢

**Objective**: Verify Opencode sessions can read work from Beads mailbox.

**Assumptions to Test**:
- Plugin can trigger `gt mail check --inject`
- Opencode agent processes mail content
- Work state syncs via Beads

**Test Procedure**:
```bash
# Setup
cd /tmp/exp-003-mailbox
gt install . --git
gt rig add test-rig https://github.com/example/repo.git

# Send mail to test address
bd create --type=mail --title="Test Work" --body="Do thing X" --to="test-rig/crew/test"

# Start Opencode session
cd test-rig/crew/test
export GT_ROLE=crew
opencode

# Observe if mail is auto-injected
# Check agent response to mail
```

**Success Criteria**:
- [ ] Mail appears in Opencode session context
- [ ] Agent acknowledges receiving work
- [ ] Agent can execute work based on mail
- [ ] Work completion updates Beads

**Data to Collect**:
- Mail injection latency
- Whether injection happens on startup or via plugin event
- Format of injected content
- Agent understanding of mail format

**Expected Outcome**: Work assignment works same as Claude

**Risk if Failed**: Need alternative work assignment mechanism

**Status**: ⬜ Not Started

---

### EXP-004: Session State Detection 🟢

**Objective**: Determine how to detect when an Opencode session is ready.

**Assumptions to Test**:
- Opencode emits a ready signal
- Can detect via stdout, file, or API
- Ready state is reliable

**Test Procedure**:
```bash
# Test 1: Check stdout for ready pattern
opencode 2>&1 | tee /tmp/opencode-stdout.log
# Look for patterns like "Ready", "Listening", "Session created", etc.

# Test 2: Check for state files
find ~/.opencode -name "*state*" -o -name "*session*"
# See if Opencode writes state files

# Test 3: Check for process signals
opencode &
PID=$!
# Monitor /proc/$PID/ for changes indicating readiness

# Test 4: Network ports
lsof -p $PID
# See if Opencode opens any ports for API
```

**Success Criteria**:
- [ ] Identified reliable ready signal
- [ ] Signal appears within 5 seconds of spawn
- [ ] Works consistently across restarts
- [ ] Can be detected programmatically

**Data to Collect**:
- Ready signal format
- Time from spawn to ready
- False positive scenarios
- Failure modes

**Expected Outcome**: Clear ready signal exists (stdout or file)

**Risk if Failed**: Use fixed delay (less reliable)

**Status**: ⬜ Not Started

---

### EXP-005: Session Cleanup 🟢

**Objective**: Verify proper cleanup when terminating Opencode sessions.

**Assumptions to Test**:
- Opencode responds to SIGTERM gracefully
- Plugin cleanup hooks work (if they exist)
- No orphaned processes or temp files

**Test Procedure**:
```bash
# Start session
cd /tmp/exp-005-cleanup
opencode &
PID=$!

# Do some work
# ...

# Terminate gracefully
kill -TERM $PID
wait $PID

# Check for cleanup
ps aux | grep opencode  # Should be empty
ls -la /tmp/opencode-*  # Check temp files
ls -la ~/.opencode/     # Check state files

# Test 2: Hard kill
opencode &
PID=$!
kill -KILL $PID
# Check for orphans and cleanup
```

**Success Criteria**:
- [ ] Graceful shutdown completes within 5 seconds
- [ ] No orphaned processes
- [ ] Temp files cleaned up
- [ ] State files in consistent state (not corrupted)
- [ ] Can restart after clean shutdown

**Data to Collect**:
- Shutdown time
- Cleanup completeness
- Edge cases (during work execution)
- Recovery after forced kill

**Expected Outcome**: Clean shutdown, no manual cleanup needed

**Risk if Failed**: Need cleanup scripts in `gt` commands

**Status**: ⬜ Not Started

---

## Advanced Experiments (Affects Design)

### EXP-006: Remote Session Creation 🟡

**Objective**: Test methods for spawning Opencode on remote hosts.

**Assumptions to Test**:
- SSH-based spawn works
- Remote session state is accessible
- Latency is acceptable

**Test Procedure**:
```bash
# Setup: Remote host with Opencode installed
# Remote: remote-host.example.com

# Test 1: SSH spawn
ssh user@remote-host "cd /workspace && opencode" &
# Verify session starts

# Test 2: SSH with state monitoring
ssh user@remote-host "cd /workspace && opencode --verbose > /tmp/opencode.log 2>&1" &
ssh user@remote-host "tail -f /tmp/opencode.log"
# Monitor startup

# Test 3: Work assignment over SSH
ssh user@remote-host "cd /workspace && bd create --type=mail --to=..."
# Verify mail appears in remote session

# Test 4: Status querying
ssh user@remote-host "cd /workspace && gt agents"
# Verify can query remote state
```

**Success Criteria**:
- [ ] Can spawn remote session reliably
- [ ] Can monitor remote session state
- [ ] Can assign work remotely
- [ ] Latency < 2 seconds for commands
- [ ] Works with key-based auth (no password prompts)

**Data to Collect**:
- Spawn latency (local vs remote)
- State query latency
- Work assignment latency
- Network failure modes
- SSH connection overhead

**Expected Outcome**: SSH-based remote works acceptably

**Risk if Failed**: Remote orchestration limited or requires API

**Status**: ⬜ Not Started

---

### EXP-007: Session Resume 🟡

**Objective**: Determine if Opencode supports session resumption.

**Assumptions to Test**:
- Opencode can resume previous sessions
- Session ID persists across restarts
- Work state is preserved

**Test Procedure**:
```bash
# Start session
cd /tmp/exp-007-resume
opencode
# Note session ID (if shown)
# Do some work, then exit

# Check for session ID storage
env | grep -i session
ls -la ~/.opencode/ | grep -i session
cat ~/.opencode/*/state.json 2>/dev/null

# Try to resume
opencode --resume <session-id>  # If supported
# or
opencode --continue
# or
opencode  # (auto-resume last?)

# Verify work state preserved
```

**Success Criteria**:
- [ ] Session ID is stored somewhere
- [ ] Can resume with session ID
- [ ] Work state (files, context) preserved
- [ ] Agent remembers previous conversation

**Data to Collect**:
- Resume mechanism (flag, auto, manual)
- What gets preserved
- What gets reset
- Session ID format

**Expected Outcome**: Resume supported (like Claude)

**Risk if Failed**: Each session is fresh start (acceptable but less ideal)

**Status**: ⬜ Not Started

---

### EXP-008: Cross-Session Messaging 🟡

**Objective**: Test message passing between Opencode sessions.

**Assumptions to Test**:
- Beads mail works across Opencode sessions
- Sessions can notify each other
- Message latency is acceptable

**Test Procedure**:
```bash
# Start session A
cd /tmp/exp-008-session-a
export GT_ROLE=polecat
export GT_AGENT_NAME=agent-a
opencode &

# Start session B
cd /tmp/exp-008-session-b
export GT_ROLE=polecat
export GT_AGENT_NAME=agent-b
opencode &

# From session A, send mail to B
# (In A's terminal)
gt mail send --to=agent-b --subject="Hello" --body="Message from A"

# Monitor B's logs/output
# Check if message appears

# Test 2: Broadcast
gt nudge --channel=workers "All polecats: status check"
# Verify both A and B receive nudge
```

**Success Criteria**:
- [ ] Mail from A appears in B
- [ ] Latency < 1 second for local sessions
- [ ] Both directions work (A→B, B→A)
- [ ] Broadcast to multiple sessions works
- [ ] Messages don't get lost

**Data to Collect**:
- Message delivery latency
- Delivery reliability (% received)
- Message format in recipient session
- Failure modes (session down, workspace moved)

**Expected Outcome**: Messaging works via Beads/filesystem

**Risk if Failed**: Need alternative messaging layer

**Status**: ⬜ Not Started

---

### EXP-009: Resource Limits 🟡

**Objective**: Determine practical limits for concurrent Opencode sessions.

**Assumptions to Test**:
- System can handle 20+ sessions
- Resource usage scales linearly
- No unexpected limits (file descriptors, etc.)

**Test Procedure**:
```bash
# Spawn sessions incrementally
for i in {1..30}; do
    (cd /tmp/exp-009-session-$i && opencode > /dev/null 2>&1) &
    PIDS[$i]=$!
    echo "Spawned session $i (PID ${PIDS[$i]})"
    
    # Measure resources
    ps aux | grep opencode | wc -l
    free -m
    uptime
    
    sleep 2
done

# Monitor for 5 minutes
# Watch for failures, slowdowns, OOM

# Assign work to all sessions
for i in {1..30}; do
    gt mail send --to=session-$i --body="Task $i"
done

# Monitor work completion
# Check if all sessions respond

# Cleanup
for pid in ${PIDS[@]}; do
    kill $pid
done
```

**Success Criteria**:
- [ ] Can spawn 20 sessions reliably
- [ ] 30 sessions possible (if sufficient RAM)
- [ ] No unexpected failures at specific counts
- [ ] Response time degrades gracefully
- [ ] System remains stable

**Data to Collect**:
- Memory per session (idle, working)
- CPU per session
- Total system load at N sessions
- First failure point (session count)
- Bottleneck identification (RAM, CPU, I/O, fds)

**Expected Outcome**: 20-25 sessions feasible on modern machine

**Risk if Failed**: Need to set conservative limits (10-15)

**Status**: ⬜ Not Started

---

## Blocking Experiments (Must Resolve)

### EXP-010: Plugin Event Catalog 🔴

**Objective**: Document all available Opencode plugin events.

**Assumptions to Test**:
- Event catalog is documented or discoverable
- Events cover needed lifecycle hooks
- Events fire reliably

**Test Procedure**:
```bash
# Create test plugin that logs all events
cat > .opencode/plugin/logger.js << 'EOF'
export const Logger = async ({ $ }) => {
  return {
    event: async ({ event }) => {
      console.log(`[LOGGER] Event: ${JSON.stringify(event, null, 2)}`);
    },
    // Try other possible hooks
    beforeCommand: async ({ command }) => {
      console.log(`[LOGGER] Before: ${command}`);
    },
    afterCommand: async ({ command, result }) => {
      console.log(`[LOGGER] After: ${command} - ${result}`);
    },
  };
};
EOF

# Run Opencode with logging plugin
opencode --verbose 2>&1 | tee /tmp/plugin-events.log

# Do various actions
# - Start session
# - Execute commands
# - Close session
# - Resume session

# Analyze log for all event types
grep "\\[LOGGER\\]" /tmp/plugin-events.log | sort -u
```

**Success Criteria**:
- [ ] Identified all available events
- [ ] Found equivalent of Claude's SessionStart
- [ ] Found equivalent of Claude's Compaction (or alternative)
- [ ] Documented event payloads
- [ ] Confirmed events fire consistently

**Data to Collect**:
- Complete event catalog
- Event payloads (structure, fields)
- Event timing/order
- Missing hooks (compared to Claude)

**Expected Outcome**: Sufficient events for Gastown integration

**Risk if Failed**: **BLOCKING** - Need to request features or use workarounds

**Status**: ⬜ Not Started

---

### EXP-011: Plugin State Persistence 🔴

**Objective**: Test if plugins can persist state across session restarts.

**Assumptions to Test**:
- Plugins can write to filesystem
- State survives session restart
- No permission issues

**Test Procedure**:
```bash
# Create stateful plugin
cat > .opencode/plugin/stateful.js << 'EOF'
export const Stateful = async ({ $, directory }) => {
  const stateFile = `${directory}/.opencode/plugin-state.json`;
  
  let state = {};
  try {
    const data = await $.fs.readFile(stateFile, 'utf8');
    state = JSON.parse(data);
  } catch (e) {
    state = { runCount: 0 };
  }
  
  return {
    event: async ({ event }) => {
      if (event?.type === 'session.created') {
        state.runCount += 1;
        state.lastRun = new Date().toISOString();
        await $.fs.writeFile(stateFile, JSON.stringify(state, null, 2));
        console.log(`[STATEFUL] Run count: ${state.runCount}`);
      }
    },
  };
};
EOF

# Run session multiple times
opencode  # Should print "Run count: 1"
# Exit
opencode  # Should print "Run count: 2"
# Exit
opencode  # Should print "Run count: 3"

# Verify state file
cat .opencode/plugin-state.json
```

**Success Criteria**:
- [ ] Plugin can write to filesystem
- [ ] State file persists across sessions
- [ ] No permission errors
- [ ] State reads back correctly
- [ ] Works with multiple concurrent sessions (no corruption)

**Data to Collect**:
- File locations (where plugins can write)
- Permission requirements
- File locking (if concurrent access)
- Performance (read/write time)

**Expected Outcome**: Plugins can maintain persistent state

**Risk if Failed**: **BLOCKING** - Can't track session-specific state

**Status**: ⬜ Not Started

---

### EXP-012: Remote API Discovery 🔴

**Objective**: Determine if Opencode has a remote API or if SSH is the only option.

**Assumptions to Test**:
- Opencode may have HTTP/RPC API
- API may be undocumented but discoverable
- API is practical for remote orchestration

**Test Procedure**:
```bash
# Check if Opencode opens network ports
opencode &
PID=$!
sleep 2
lsof -p $PID | grep LISTEN
netstat -tlnp | grep $PID

# Check Opencode CLI for API-related flags
opencode --help | grep -i api
opencode --help | grep -i server
opencode --help | grep -i remote

# Check documentation
# (Review Opencode repo README, docs/)

# Check for RPC/API code
# (Search Opencode source for 'server', 'api', 'rpc', etc.)

# If API found, test basic calls
curl http://localhost:<port>/api/sessions  # Example
```

**Success Criteria**:
- [ ] Determined if API exists
- [ ] If yes: documented API endpoints
- [ ] If yes: tested basic session operations
- [ ] If no: confirmed SSH is the recommended approach

**Data to Collect**:
- API availability (yes/no)
- API documentation location
- API capabilities vs SSH capabilities
- Authentication requirements

**Expected Outcome**: Either find API or confirm SSH approach

**Risk if Failed**: **BLOCKING** - Need clear path for remote orchestration

**Status**: ⬜ Not Started

---

## Experiment Tracking

### Summary Table

| ID | Name | Priority | Status | Owner | Blocking? |
|----|------|----------|--------|-------|-----------|
| EXP-001 | Multi-Session Spawn | 🟢 Basic | ⬜ Not Started | TBD | No |
| EXP-002 | Plugin Installation | 🟢 Basic | ⬜ Not Started | TBD | No |
| EXP-003 | Work Assignment via Mailbox | 🟢 Basic | ⬜ Not Started | TBD | No |
| EXP-004 | Session State Detection | 🟢 Basic | ⬜ Not Started | TBD | No |
| EXP-005 | Session Cleanup | 🟢 Basic | ⬜ Not Started | TBD | No |
| EXP-006 | Remote Session Creation | 🟡 Advanced | ⬜ Not Started | TBD | No |
| EXP-007 | Session Resume | 🟡 Advanced | ⬜ Not Started | TBD | No |
| EXP-008 | Cross-Session Messaging | 🟡 Advanced | ⬜ Not Started | TBD | No |
| EXP-009 | Resource Limits | 🟡 Advanced | ⬜ Not Started | TBD | No |
| EXP-010 | Plugin Event Catalog | 🔴 Blocking | ⬜ Not Started | TBD | **YES** |
| EXP-011 | Plugin State Persistence | 🔴 Blocking | ⬜ Not Started | TBD | **YES** |
| EXP-012 | Remote API Discovery | 🔴 Blocking | ⬜ Not Started | TBD | **YES** |

### Status Legend
- ⬜ Not Started
- 🔄 In Progress
- ✅ Complete
- ❌ Failed
- ⚠️ Blocked

## Next Steps

1. **Assign Owners**: Determine who will run each experiment
2. **Prioritize**: Start with blocking experiments (EXP-010, EXP-011, EXP-012)
3. **Schedule**: Set target dates for each experiment
4. **Document**: Record results in this file or separate reports
5. **Review**: Update architecture based on findings

## Results Documentation Template

For each completed experiment, add results below:

```markdown
### EXP-XXX Results

**Date**: YYYY-MM-DD
**Owner**: Name
**Duration**: X hours

**Findings**:
- Key finding 1
- Key finding 2
- ...

**Data**:
- Metric 1: Value
- Metric 2: Value
- ...

**Conclusion**: 
Brief summary of outcome and impact on design.

**Follow-up Actions**:
- [ ] Action 1
- [ ] Action 2
```

---

**Last Updated**: 2026-01-15
**Owner**: Gastown Team
**Next Review**: After completing blocking experiments
