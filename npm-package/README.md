# @gastown/gt

Gas Town CLI - multi-agent workspace manager for coordinating AI coding agents.

## Installation

```bash
npm install -g @gastown/gt
```

This package downloads the matching native binary from the Gas Town GitHub
Releases page during installation.

## Channel Expectations

- GitHub Releases are the authoritative source for Gas Town community releases.
- `@gastown/gt` is a convenience wrapper around those GitHub release binaries.
- npm publishing is best-effort, so `npm view @gastown/gt version` can lag the
  newest GitHub release.
- If you need the newest published release immediately, verify the GitHub
  Releases page first and fall back to downloading the binary directly.

## Usage

```bash
# Check version
gt version

# Initialize a new town
gt init

# View status
gt status

# List rigs
gt rig list
```

## Supported Platforms

- macOS (Intel and Apple Silicon)
- Linux (x64 and ARM64)
- Windows (x64)

## Manual Installation

If npm installation fails, you can download binaries directly from:
https://github.com/steveyegge/gastown/releases

## License

MIT
