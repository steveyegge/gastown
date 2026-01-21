# ESP32S3 Gas Town Agent Specifications

## Overview

This document contains specifications and testing requirements for the Gas Town AI orchestrator when used with ESP32S3 devices. The Gas Town system provides automated AI agent workflows with support for multiple AI runtimes including Claude, OpenCode, Gemini, and others.

## OpenCode Agent Integration

### Status: ✅ IMPLEMENTED

The OpenCode CLI agent has been successfully integrated into Gas Town's built-in agent registry.

#### Configuration Details

- **Agent Name**: `opencode`
- **Command**: `opencode`
- **Default Args**: `[]` (empty, no default arguments)
- **Process Names**: `["opencode"]`
- **Session ID Environment Variable**: `OPENCODE_SESSION_ID`
- **Resume Flag**: `--resume`
- **Resume Style**: `flag`
- **Supports Hooks**: `true`
- **Supports Fork Session**: `false`

#### Usage Examples

```bash
# Use OpenCode as the default agent
gt town agent opencode

# Start a session with OpenCode
gt start --agent opencode

# Resume an OpenCode session
gt resume --session-id <SESSION_ID>
```

#### Session Management

OpenCode sessions are managed using the `OPENCODE_SESSION_ID` environment variable. Sessions can be resumed across restarts using the `--resume` flag.

#### Hook Integration

OpenCode supports the Gas Town hooks system for:
- Pre-session initialization
- Post-session cleanup
- Custom workflow integrations

## Testing Requirements

### Unit Tests
- ✅ Agent preset registration in registry
- ✅ Configuration validation
- ✅ Process name detection
- ✅ Session resumption command generation

### Integration Tests (ESP32S3 Device Required)
- [ ] OpenCode CLI installation and accessibility
- [ ] Session creation and persistence
- [ ] Hook execution with OpenCode
- [ ] Resume functionality across device restarts
- [ ] Process detection in tmux environment

### Hardware-Specific Testing Notes

#### ESP32S3 Considerations
1. **Memory Constraints**: Verify OpenCode CLI memory usage fits within ESP32S3 constraints
2. **Storage**: Ensure sufficient storage for CLI binary and session data
3. **Network Connectivity**: Test OpenCode's network dependencies on ESP32S3 networking stack
4. **Power Management**: Validate power consumption during long-running sessions

#### Test Environment Setup
```bash
# On ESP32S3 development environment
export OPENCODE_SESSION_ID=test_session_123
gt start --agent opencode
# Verify process is running
ps aux | grep opencode
# Test resume
gt resume --session-id test_session_123
```

## Future Work

### Known Issues
- No fork session support (OpenCode limitation)
- Requires manual CLI installation on target device

### Enhancement Roadmap
1. **Automatic CLI Installation**: Bundle OpenCode CLI with Gas Town distribution
2. **Resource Monitoring**: Add memory and CPU usage monitoring for OpenCode sessions
3. **Device-Specific Optimizations**: Optimize OpenCode configuration for embedded devices
4. **Offline Mode**: Investigate offline capabilities for network-constrained environments

## Testing Checklist

Before deploying to ESP32S3:

- [ ] Verify OpenCode CLI binary compatibility with target architecture
- [ ] Test memory footprint under typical workload
- [ ] Validate session persistence across device reboots
- [ ] Test hook integration with device-specific workflows
- [ ] Confirm network connectivity requirements are met
- [ ] Performance benchmarking vs other agents (Claude, Gemini)

## Documentation Updates

Each successful implementation should update this README with:
1. Performance characteristics on ESP32S3
2. Memory usage statistics
3. Network requirement details
4. Any device-specific configuration adjustments
5. Lessons learned and optimization opportunities