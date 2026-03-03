## Product Spec

Add github.com/github/copilot-sdk/go (or its current module name) to the Gas Town codebase.[libraries+1](https://libraries.io/go/github.com%2Fgithub%2Fcopilot-sdk%2Fgo)
Replace the current Claude Code “worker driver” with a Go component that:
Starts or connects to the Copilot CLI server (the SDK manages this via JSON‑RPC).YouTube​[libraries](https://libraries.io/go/github.com%2Fgithub%2Fcopilot-sdk%2Fgo)​
Creates a Copilot agent instance with instructions tailored to the worker role.[github+1](https://github.blog/news-insights/company-news/build-an-agent-into-any-app-with-the-github-copilot-sdk/)
Feeds it the same crew/hook/git context that you currently pass to Claude (mail, task description, repo layout).[github+1](https://github.com/steveyegge/gastown)
Applies file edits and shell commands via the SDK’s built‑in tools (equivalent of --allow-all on the CLI).[github+1](https://github.com/github/copilot-sdk)
This lets you keep everything inside the existing Go process: no sidecar worker, no cross‑language IPC, and a direct mapping from Gas Town’s worker abstraction to a Copilot agent.

Here’s a focused product spec for “Gas Town with Copilot SDK” along the path you described.

***

## 1. Goal and scope

Replace Claude Code CLI as the default coding agent runtime in Gas Town with a **GitHub Copilot SDK–powered agent**, implemented directly in Go, without changing Gas Town’s core concepts (Mayor, rigs, crews, polecats, hooks, Beads).[^1][^2][^3]

Out of scope for v1:

- UI/UX changes to `gt` CLI beyond minimal flags/config.
- New multi-agent patterns inside Copilot (just a single Copilot agent per worker, orchestrated by existing Gas Town mechanics).[^4][^3]

***

## 2. User stories

1. **As a Gas Town user**, I can configure Copilot as my default agent so that `gt convoy` and `gt sling` end up driving Copilot instead of Claude Code.[^2][^1]
2. **As a Mayor user**, when I create convoys and sling work, workers pick up tasks and a Copilot agent reads my mail, edits code, runs tests, and pushes changes, all within the crew workspace.[^3][^1]
3. **As an operator**, I can switch between Claude and Copilot per-town or per-rig via config, to roll out Copilot incrementally.[^5][^2]
4. **As a developer**, I can build and run Gas Town with no extra sidecars: Copilot is driven via the Go SDK inside the Gas Town process, with the SDK managing the Copilot CLI server.[^6][^3]

***

## 3. Architecture \& components

### 3.1 New dependency

- Add Go module: `github.com/github/copilot-sdk/go` (exact import path as in the technical preview).[^3][^6]


### 3.2 New runtime type

Introduce a conceptual **AgentRuntime** abstraction (if not already explicit):

- `type AgentRuntime string` with values:
    - `"claude"` (existing)
    - `"copilot"` (new)

Gas Town config (e.g., `gt config`) gets:

- `gt config default-agent-runtime copilot|claude`
- Optional per-rig override: e.g., `.gastown/config.toml` with `agent_runtime = "copilot"`.[^2]


### 3.3 Copilot agent driver (Go)

Create a package, e.g. `internal/agent/copilot`:

Responsibilities:

1. **Client bootstrap**
    - Start or connect to Copilot CLI runtime via the Go SDK’s `Client` type (name TBD per SDK), which shells out to `copilot`/`gh copilot` and speaks JSON‑RPC.[^7][^6][^3]
    - Handle auth errors (e.g., missing `gh auth login` / Copilot entitlement) and surface them as friendly CLI messages.
2. **Agent creation**
    - Given a worker role (Mayor, polecat worker, etc.), construct an agent with:
        - `instructions`: description of the role and responsibilities, mirroring current Claude prompt (e.g., “You are a Gas Town *worker* operating in this repo, follow hooks, use Beads, keep changes small and testable.”).[^2][^3]
        - `tools`: enable shell \& file tools (equivalent to `--allow-all`).[^8][^3]
    - Example (pseudo-Go, matching SDK concepts):
        - `client := copilot.NewClient()`
        - `client.Start(ctx)`
        - `agent := client.AsAIAgent(ctx, tools, instructions, options)`[^9][^3]
3. **Session management**
    - For each **worker instance** (crew/polecat), maintain a Copilot **session**:

```
- Session id persisted in the worker’s directory (e.g., `.copilot-session.json`) or in memory keyed by `<rig>/<crew>/<worker-id>`.
```

        - When the worker wakes up (hook fires), it reuses the session if present; otherwise it creates one.[^9][^3]
    - Sessions should be short-lived but survive across multiple hook invocations for the same convoy to help Copilot with short-term conversational memory, while Beads/git remain the system of record.[^10][^2]
4. **Task execution loop**
    - The driver exposes a call like:
`func (r *Runtime) RunHook(ctx context.Context, workerCtx WorkerContext) error`
    - `WorkerContext` includes:
        - Paths: town root, rig root, crew dir, current worktree.[^1][^2]
        - Hook file path and contents (the “mail”/task description).[^1]
        - Beads issue ids / formulas relevant to this worker.
    - Inside `RunHook`:

5. **Assemble prompt**: include hook text, a short summary of the project (from existing Gas Town metadata), and any Mayor instructions for this worker.
6. **Call agent**: `agent.RunAsync(prompt, session)` (SDK naming tbd), requesting:
            - Tool access for shell + file ops within the crew directory.[^8][^3]
            - Streaming of text back to a log file or the terminal if running interactively.
7. **Apply changes**:
            - Allow Copilot to directly edit files and run commands via tools, but impose guardrails:
                - Working directory = crew workspace.
                - Enforce `git status` cleanliness on completion; optionally run `git diff` summarization.
8. **Persist results**:
            - Ensure all changes are committed to the worker’s worktree and Beads log entries are updated, preserving the existing Gas Town behavior.[^5][^1][^2]
1. **Shutdown / cleanup**
    - Ensure Copilot client is gracefully stopped on `gt` process exit.
    - Optionally clean/rotate `.copilot` temp directories.

***

## 4. UX \& CLI behavior

### 4.1 Installation \& prerequisites

Document new prerequisites:

- GitHub Copilot Enterprise or individual subscription with CLI/SDK entitlement.[^11][^3]
- `gh` or `copilot` CLI installed and authenticated (`gh auth login` and `gh extension install github/copilot-cli` if still required).[^7]
- Gas Town vX.Y+ compiled with Copilot SDK module.[^6][^1]

Update Quick Start:

- New variant:
    - `gt config default-agent-runtime copilot`
    - `gt doctor` checks Copilot availability (CLI + SDK).[^5][^1]


### 4.2 Running work

The canonical flow remains:

```sh
gt convoy create "Fix bugs" issue-123
gt sling issue-123 myproject
gt status   # or gt peek
```

Behavior difference when Copilot is default:

- Instead of instructions telling you to run `claude --resume`, Gas Town will automatically start a Copilot-powered worker process for that crew, using the Go SDK inside `gt` itself (or via `gt worker` subcommand).[^3][^1][^2]

For minimal/manual mode, optionally allow:

```sh
gt worker copilot --rig myproject --crew yourname
```

which runs one Copilot-worker loop in the current terminal for the selected crew.

***

## 5. Configuration model

Add fields to Gas Town config (conceptual):

```toml
[agent]
default_runtime = "copilot"   # or "claude"

[agent.copilot]
instructions_template = "You are a Gas Town worker..."  # overrideable
allow_shell = true
allow_file_edits = true
max_tokens = 4000
model = "gpt-4.1"  # or other Copilot-supported models
```

- Allow per-rig override (`rig/.gastown/config.toml`) to experiment per project.[^2]
- Optionally support per-role instructions (Mayor vs polecats vs Refinery) in the same config file.

***

## 6. Security and permissions

- Use Copilot SDK’s permission hooks to **intercept file/shell tool invocations** and optionally:
    - Auto‑approve within the crew workspace.
    - Deny writes outside the rig directory.
    - Log all shell commands and file edits to a worker audit log (Beads or a `.logs/` dir).[^9][^8][^3]
- `gt doctor` should warn if:
    - Copilot is not enabled for the user/org.
    - CLI is missing or outdated.
    - SDK cannot start the Copilot runtime.

***

## 7. Compatibility \& migration

- Keep existing Claude integration intact:
    - Default remains Claude unless `default_runtime` is explicitly set to `copilot`.
    - `gt config default-agent` and any existing `gt config agent set` commands continue to work; they now pick a “profile” that is mapped to either a Claude or Copilot runtime under the hood.[^2]
- For Beads:
    - No change to formulas, molecules, or issue semantics. Workers are still responsible for cooking formulas and updating molecules; they just do it via Copilot rather than Claude.[^1][^5]

***

## 8. Acceptance criteria

1. **Config \& startup**
    - `gt doctor` passes with Copilot enabled and indicates “Copilot SDK available”.[^6][^1]
    - `gt config default-agent-runtime copilot` persists and is honored on new convoys.
2. **Worker execution**
    - When a convoy is slung and a worker awakens, a Copilot session is created and:
        - Reads the hook/mail.
        - Edits files and runs commands in the crew workspace.
        - Leaves the repo in a consistent state with committed changes or clearly logged failures.[^3][^1][^2]
3. **Parity with Claude minimal mode**
    - A simple “Fix bugs in file X” convoy results in a PR-ready change using Copilot, with no manual `claude --resume` step.[^1][^2]
    - Switching back to `default_runtime = "claude"` restores previous behavior.
4. **Safety**
    - No writes occur outside the rig directory.
    - All shell commands and edits invoked by Copilot are recorded in a worker log.

***

## 9. Future enhancements (post‑v1)

- Add support for **multi-agent orchestrations** behind a single worker by leveraging Copilot’s MCP + tools more heavily.[^8][^3]
- Integrate **Microsoft Agent Framework** as an alternative runtime that uses Copilot SDK but offers richer multi-agent patterns, exposed as another `AgentRuntime` (e.g., `"copilot-mf"`).[^9]
- Build a small TUI under Mayor to live‑view Copilot agent streams per worker.

If you’d like, I can next turn this into a concrete Go interface + skeleton implementation for `internal/agent/copilot` that matches the current Gastown module layout on pkg.go.dev.[^1][^2]
<span style="display:none">[^12][^13][^14][^15][^16][^17][^18]</span>

<div align="center">⁂</div>

[^1]: https://pkg.go.dev/github.com/ctiospl/gastown@v0.3.0

[^2]: https://pkg.go.dev/github.com/cosmiccareer/gastown@v0.0.0-20260120143722-0b14c1c4584e

[^3]: https://github.com/github/copilot-sdk

[^4]: https://www.linkedin.com/posts/satyanadella_build-an-agent-into-any-app-with-the-github-activity-7420126187286568961-TdW7

[^5]: https://steveyegge.github.io/beads/integrations/claude-code

[^6]: https://libraries.io/go/github.com%2Fgithub%2Fcopilot-sdk%2Fgo

[^7]: https://www.youtube.com/watch?v=GsEPS1yHaHQ

[^8]: https://techcommunity.microsoft.com/blog/azuredevcommunityblog/building-agents-with-github-copilot-sdk-a-practical-guide-to-automated-tech-upda/4488948

[^9]: https://devblogs.microsoft.com/semantic-kernel/build-ai-agents-with-github-copilot-sdk-and-microsoft-agent-framework/

[^10]: https://maggieappleton.com/gastown

[^11]: https://github.com/features/copilot

[^12]: https://support.claude.com/en/articles/10167454-using-the-github-integration

[^13]: https://www.youtube.com/watch?v=y5lLyzZhNDQ

[^14]: https://claudecodeplugins.io/skills/gastown/

[^15]: https://nextbuild.in/blog/build-an-agent-into-any-app-with-the-github-copilot-sdk

[^16]: https://github.com/anthropics/claude-code

[^17]: https://www.youtube.com/watch?v=3x0X85qtWyM

[^18]: https://github.com/features/copilot/agents

