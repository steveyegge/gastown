# OpenCode Integration Maintenance Guide

> **Audience**: Gastown maintainers  
> **Purpose**: Operational guide for maintaining OpenCode integration  
> **Status**: Community-maintained feature

## Overview

OpenCode support is a community-contributed, experimental feature that extends Gastown's runtime abstraction. This guide covers maintenance procedures, update strategies, and troubleshooting.

## Maintenance Responsibilities

### Primary Maintainer
- **Status**: Community-maintained
- **Contact**: Via GitHub issues with `opencode` label
- **Support Level**: Best-effort, community-driven

### Core Team Responsibilities
- Review OpenCode-related PRs for code quality and architectural fit
- Ensure OpenCode changes don't impact Claude Code (default runtime)
- Maintain runtime abstraction layer that OpenCode depends on
- Not responsible for OpenCode CLI bugs or features

## Version Compatibility

### OpenCode CLI Version
- **Current Tested**: v1.1.25
- **Minimum Required**: v1.1.0 (estimated)
- **Update Cadence**: As needed when breaking changes occur

### Compatibility Matrix

| Gastown Version | OpenCode CLI | Status | Notes |
|-----------------|--------------|--------|-------|
| 0.2.6+ | v1.1.25 | ✅ Tested | Initial integration |
| Future | v1.2.x | ⚠️ Unknown | Will need testing |

## Update Procedures

### When OpenCode CLI Updates

**Minor Version (1.1.x → 1.2.x)**:
1. Review OpenCode release notes for breaking changes
2. Check plugin API compatibility
3. Run test suite: `go test ./internal/opencode/... ./internal/config/...`
4. Update version in documentation if changed
5. Test integration scenarios from `docs/opencode/archive/e2e-test-results.md`

**Major Version (1.x → 2.x)**:
1. Thorough review of breaking changes
2. Update plugin implementation if event system changed
3. Update all documentation
4. Full regression testing
5. Consider creating migration guide

### When Gastown Updates

**Before Each Release**:
1. Run OpenCode test suite: `go test ./internal/opencode/...`
2. Verify runtime abstraction tests pass
3. Check that default behavior (Claude) unchanged
4. Optional: Manual smoke test with OpenCode CLI if available

**Breaking Changes to Runtime Abstraction**:
1. Update OpenCode plugin if affected
2. Update OpenCode documentation
3. Add migration notes to OpenCode docs

## Testing Strategy

### Required Tests (CI)
- `go test ./internal/opencode/...` - Always run
- `go test ./internal/config/...` - Includes OpenCode config tests
- `go test ./internal/runtime/...` - Runtime abstraction tests

### Optional Tests (Manual)
- Integration tests require OpenCode CLI installed
- Can be skipped if CLI not available
- Run before releases when possible

### Test Automation
```bash
# Minimal CI test (no OpenCode CLI required)
go test ./internal/opencode/... ./internal/config/...

# Full integration test (requires OpenCode CLI)
./scripts/test-opencode-integration.sh
```

## Common Issues and Solutions

### Issue: Plugin Not Loading

**Symptoms**: OpenCode session starts but hooks don't fire

**Diagnosis**:
```bash
# Check if plugin installed
ls -la .opencode/plugin/gastown.js

# Check OpenCode config
cat ~/.config/opencode/opencode.jsonc | grep plugin
```

**Solution**:
```bash
# Reinstall plugin
gt prime  # Reinstalls plugin via runtime.EnsureSettingsForRole
```

### Issue: Session Fork Not Working

**Symptoms**: HTTP API fork returns error

**Diagnosis**:
- Check if `opencode serve` is running
- Verify session exists: `opencode session list`
- Check HTTP endpoint: `curl http://localhost:4096/health`

**Solution**: See `docs/opencode/archive/session-fork-test-results.md`

### Issue: Model Not Available

**Symptoms**: OpenCode can't access certain models

**Diagnosis**:
- Check auth provider: `opencode models`
- Verify provider configuration in `~/.config/opencode/opencode.jsonc`

**Solution**: This is an OpenCode CLI configuration issue, not a Gastown issue. Refer user to OpenCode documentation.

## Deprecation Strategy

### If OpenCode Becomes Unmaintainable

**Criteria for Deprecation**:
- No community maintainer for 6+ months
- Breaking changes in OpenCode CLI with no fix
- Significant security vulnerabilities
- Maintenance burden outweighs benefit

**Deprecation Process**:
1. Mark as deprecated in README and docs
2. Add deprecation warning to OpenCode preset
3. Keep code for 1-2 releases
4. Remove in major version bump (e.g., 1.0.0)

**Current Status**: ✅ Active, community-maintained

## Support Policy

### What's Supported
- ✅ Core runtime abstraction (used by all agents)
- ✅ OpenCode agent preset configuration
- ✅ Plugin embedding and installation
- ✅ Integration with Gastown features (Beads, tmux, etc.)

### What's Not Supported
- ❌ OpenCode CLI bugs or features
- ❌ OpenCode authentication provider issues
- ❌ Model availability or performance
- ❌ OpenCode HTTP API bugs

### User Support Flow
1. **OpenCode CLI Issues**: Direct to OpenCode GitHub issues
2. **Integration Issues**: GitHub issues with `opencode` label
3. **Runtime Abstraction Issues**: Standard Gastown support

## Monitoring and Metrics

### Usage Tracking (Optional)
- Monitor GitHub issues with `opencode` label
- Track community contributions to OpenCode integration
- Survey user adoption if interested

### Health Indicators
- ✅ All tests passing
- ✅ No open critical issues
- ✅ Community engagement (PRs, issues)
- ⚠️ OpenCode CLI actively maintained upstream

## Contact and Resources

### Internal Resources
- Runtime abstraction: `internal/runtime/runtime.go`
- OpenCode plugin: `internal/opencode/plugin.go`
- Agent presets: `internal/config/agents.go`

### External Resources
- OpenCode Repository: https://github.com/anomalyco/opencode
- OpenCode Documentation: https://opencode.ai
- Community Forum: GitHub Discussions (if enabled)

### Maintenance Questions
- Create GitHub issue with `opencode` and `maintenance` labels
- Mention `@community-maintainer` if assigned

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-18 | 1.0 | Initial maintenance guide |

---

**Note**: This guide will evolve as maintenance patterns emerge. Update as needed based on real-world maintenance experience.
