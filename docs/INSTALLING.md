# Installing Gas Town

Gas Town now has one canonical install story for end users and one canonical
story for contributors:

- End users on macOS/Linux: use Homebrew and keep using Homebrew for upgrades.
- Contributors and local source builds: clone the repo and use `make install`.
- Optional npm installs are available, but they are not the canonical upgrade path
  and may lag Homebrew or GitHub releases.
- Avoid `go install github.com/steveyegge/gastown/cmd/gt@latest` for `gt` itself.
  It drops a binary into `~/go/bin`, which is the shadowing/stale-binary path that
  this guide is trying to prevent.

## Choose One Install Channel

| Channel | Use this when | Install | Upgrade | Notes |
|---------|---------------|---------|---------|-------|
| **Homebrew** | You want the standard end-user path on macOS/Linux | `brew install gastown` | `brew upgrade gastown` | Recommended. Current Homebrew formula pulls in `bd` and `dolt` transitively. |
| **Source / dev build** | You are working from a local checkout or hacking on Gas Town itself | `git clone ... && cd gastown && make install` | `git pull --ff-only && make install` | Canonical source path. Installs `gt` to `~/.local/bin/gt` and removes stale `~/go/bin/gt` and `~/bin/gt`. |
| **npm** | You specifically want the Node-distributed CLI | `npm install -g @gastown/gt` | `npm update -g @gastown/gt` | Optional convenience path. May lag other release channels and does not manage `bd` or `dolt` for you. |
| **Windows / manual build** | You are on Windows or want to manage binaries yourself | `go build -o gt.exe ./cmd/gt` | Rebuild from the updated checkout | Treat this like the source path and make sure the resulting binary is the one your shell resolves. |

## Required Tools

| Tool | Minimum | Needed for | Notes |
|------|---------|------------|-------|
| **Homebrew** | latest | Homebrew install path | Recommended distribution channel on macOS/Linux |
| **Go** | 1.25+ | Source/dev builds, Windows/manual builds, manual `bd` installs | Not required if Homebrew is managing your binaries |
| **Git** | 2.25+ | Worktree support, rig management | Required for real-world use |
| **Dolt** | 1.82.4+ | Town and beads storage | Homebrew installs it transitively with `gastown` today |
| **beads (bd)** | 0.57.0+ | All normal installs unless you intentionally use `gt install --no-beads` | Homebrew installs it transitively with `gastown` today |
| **tmux** | 3.0+ | Full stack mode | Optional but recommended |
| **sqlite3** | latest | Convoy database queries | Usually preinstalled on macOS/Linux |

Optional runtimes for actual agent work:

- Claude Code CLI (default)
- Codex CLI
- OpenCode CLI
- GitHub Copilot CLI

## Homebrew Install

This is the canonical end-user path on macOS/Linux.

```bash
brew install gastown

gt version
bd version
dolt version
```

When you want a newer community release:

```bash
brew upgrade gastown
```

## Source / Dev Install

This is the canonical path when you are working from a checkout.

```bash
git clone https://github.com/steveyegge/gastown.git
cd gastown
make install

# Ensure the canonical source-install location is on PATH
export PATH="$HOME/.local/bin:$PATH"

# Install beads yourself if you want it before the first gt install,
# or let gt install auto-install it later if Go is available.
GOBIN="$HOME/.local/bin" go install github.com/steveyegge/beads/cmd/bd@latest

gt version
bd version
```

`make install` is intentionally different from `go install`:

- It writes `gt` to `~/.local/bin/gt`.
- It removes stale `~/go/bin/gt` and `~/bin/gt` copies that can shadow the
  binary you meant to run.
- It preserves the expected local-dev workflow used by `gt stale`, `gt doctor`,
  and the source tree itself.

To upgrade a source install:

```bash
git pull --ff-only
make install
```

## npm Install

The npm package is supported, but it is not the canonical upgrade story.
Use it when you explicitly want the Node-distributed CLI.

```bash
npm install -g @gastown/gt
```

You still need `bd` and `dolt` available. One simple path is:

```bash
GOBIN="$HOME/.local/bin" go install github.com/steveyegge/beads/cmd/bd@latest
```

To upgrade:

```bash
npm update -g @gastown/gt
```

## Windows / Manual Build

Gas Town can be built directly from a checkout on Windows:

```bash
git clone https://github.com/steveyegge/gastown.git
cd gastown
go build -o gt.exe ./cmd/gt
```

Put `gt.exe` somewhere stable and make sure that directory is on `PATH`.
When you upgrade, rebuild from the updated checkout and verify the resolved path
again.

## PATH and Shadowing

Gas Town will run whichever `gt` comes first on `PATH`, not whichever one you
installed most recently.

Common locations:

- Homebrew: `/usr/local/bin/gt` or `/opt/homebrew/bin/gt`
- Source/dev install: `~/.local/bin/gt`
- Old `go install` copy: `~/go/bin/gt`
- Manual helper copy: `~/bin/gt`

After every install, upgrade, or channel switch, verify the active binary:

```bash
command -v gt
which -a gt    # or: type -a gt
gt version
bd version
```

On Windows, use:

```powershell
where.exe gt
gt version
```

If the wrong binary is winning:

- Remove or rename the stale copy that appears earlier on `PATH`.
- Update your shell config so the intended directory comes first.
- Restart the shell or run `hash -r` / `rehash`.
- Run `gt doctor` after the switch.

## Create Your Workspace

Once `gt` resolves to the binary you intend to use:

```bash
gt install ~/gt --shell --git
cd ~/gt

gt rig add myproject https://github.com/you/repo.git
gt doctor
gt status
```

That creates:

```text
~/gt/
├── CLAUDE.md
├── mayor/
├── rigs/
└── .beads/
```

## First Steps After Install

```bash
gt config agent list
gt mayor attach
```

If you want tmux-managed agents and background patrols:

```bash
gt daemon start
gt up
```

## Updating

Pick the command that matches the channel you chose originally:

```bash
# Homebrew
brew upgrade gastown

# Source / dev build
git pull --ff-only
make install

# npm
npm update -g @gastown/gt
```

Then always verify the active binary and repair any post-upgrade state:

```bash
command -v gt
which -a gt  # or: type -a gt
gt version
gt doctor --fix
```

## Troubleshooting

### `gt: command not found`

For source/dev installs, ensure `~/.local/bin` is on `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
source ~/.zshrc  # or ~/.bashrc
```

If you used npm, ensure your npm global bin directory is on `PATH`.

### `bd: command not found`

If you are on Homebrew, reinstall or repair the Homebrew package:

```bash
brew reinstall gastown
```

Otherwise install `bd` into the same canonical local bin directory used by the
source/dev path:

```bash
GOBIN="$HOME/.local/bin" go install github.com/steveyegge/beads/cmd/bd@latest
```

### `gt doctor` shows errors

```bash
gt doctor --fix
```

If the report still looks wrong after an install or upgrade, double-check the
resolved `gt` path first. Many "upgrade didn't work" reports are actually PATH
shadowing.

### Daemon not starting

```bash
tmux -V
tmux new-session -d -s test && tmux kill-session -t test
```

### Git authentication issues

```bash
ssh -T git@github.com
git config --global credential.helper cache
```

## Uninstalling

Use the built-in cleanup path instead of manually deleting random files:

```bash
gt uninstall
```

If you also want to remove the workspace:

```bash
gt uninstall --workspace
```

## Next Steps

1. Read the [README](../README.md) for concepts and workflows.
2. Use [docs/reference.md](reference.md) for command reference.
3. Run `gt doctor` after upgrades or channel switches.
4. Use `gt wl join hop/wl-commons` if you want to explore the Wasteland federation.
