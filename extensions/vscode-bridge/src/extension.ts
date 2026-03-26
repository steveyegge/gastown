/**
 * Gas Town Bridge Extension — main entry point
 *
 * Bridges Gas Town multi-agent orchestration with GitHub Copilot:
 * 1. Terminal Event Hook Emitter — writes .events.jsonl on command execution
 * 2. Heartbeat Watcher — monitors Deacon heartbeat, shows status in status bar
 * 3. Session Context Tools — save/load session summaries for Seance-like recovery
 * 4. Headless Session Lifecycle — registers session, emits heartbeat, watches nudges
 * 5. Nudge Watcher — receives nudges via FileSystemWatcher (replaces tmux send-keys)
 * 6. Heartbeat Emitter — signals liveness to daemon/witness (replaces tmux.HasSession)
 */
import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import { TerminalHookEmitter } from "./terminal-hooks";
import { HeartbeatWatcher } from "./heartbeat-watcher";
import { SessionManager } from "./session-manager";
import { NudgeWatcher } from "./nudge-watcher";
import { HeartbeatEmitter } from "./heartbeat-emitter";

/** Generate a unique session ID for this VS Code window. */
function generateSessionId(): string {
  return `ghcp-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}

export function activate(context: vscode.ExtensionContext): void {
  const config = vscode.workspace.getConfiguration("gastown");
  const townRoot = resolveTownRoot(config);

  // Log activation
  const outputChannel = vscode.window.createOutputChannel("Gas Town Bridge");
  outputChannel.appendLine(`Gas Town Bridge activated. Town root: ${townRoot}`);
  context.subscriptions.push(outputChannel);

  // 1. Terminal Hook Emitter
  if (config.get<boolean>("emitTerminalEvents", true)) {
    const hooks = new TerminalHookEmitter(townRoot);
    context.subscriptions.push(hooks);
  }

  // 2. Heartbeat Watcher + Status Bar (monitors Deacon)
  if (config.get<boolean>("heartbeatWatchEnabled", true)) {
    const watcher = new HeartbeatWatcher(townRoot, context);
    context.subscriptions.push(watcher);
  }

  // 3. Session Context Commands
  const sessions = new SessionManager(townRoot);
  context.subscriptions.push(
    vscode.commands.registerCommand("gastown.showStatus", () => sessions.showStatus()),
    vscode.commands.registerCommand("gastown.saveSession", () => sessions.saveSession()),
    vscode.commands.registerCommand("gastown.loadSession", () => sessions.loadSession()),
  );

  // 4. Headless Session Lifecycle — register this VS Code window as a Gas Town session
  const sessionId = context.workspaceState.get<string>("gastown.sessionId") ?? generateSessionId();
  context.workspaceState.update("gastown.sessionId", sessionId);
  outputChannel.appendLine(`Session ID: ${sessionId}`);

  // Write session descriptor to .runtime/sessions/ (same format as startHeadlessSession in Go)
  registerHeadlessSession(townRoot, sessionId, outputChannel);

  // 5. Heartbeat Emitter — signals liveness to daemon and witness
  const heartbeat = new HeartbeatEmitter(townRoot, sessionId);
  context.subscriptions.push(heartbeat);

  // 6. Nudge Watcher — receives nudges via native file system watcher
  const nudgeWatcher = new NudgeWatcher(townRoot, sessionId);
  context.subscriptions.push(nudgeWatcher);

  // 7. Emit session_start event to .events.jsonl
  emitSessionStart(townRoot, sessionId);

  // Expose session ID for other extensions / MCP tools
  context.subscriptions.push(
    vscode.commands.registerCommand("gastown.getSessionId", () => sessionId),
  );

  outputChannel.appendLine(`Headless session active: nudge watcher + heartbeat emitter running`);
}

export function deactivate(): void {
  // cleanup handled by disposables
}

function resolveTownRoot(config: vscode.WorkspaceConfiguration): string {
  const explicit = config.get<string>("townRoot", "");
  if (explicit) return explicit;
  const home = process.env.USERPROFILE ?? process.env.HOME ?? "";
  return path.join(home, "gt");
}

/** Register this VS Code window as a headless Gas Town session. */
function registerHeadlessSession(townRoot: string, sessionId: string, log: vscode.OutputChannel): void {
  const sessionsDir = path.join(townRoot, ".runtime", "sessions");
  try {
    fs.mkdirSync(sessionsDir, { recursive: true });

    // Detect rig from workspace
    const rig = detectRigFromWorkspace();

    // Write JSON session descriptor
    const descriptor = {
      session_id: sessionId,
      run_id: sessionId, // For headless, session_id doubles as run_id
      role: "polecat",
      rig,
      agent: "copilot",
      work_dir: vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? "",
      headless: true,
      started_at: new Date().toISOString(),
      platform: "vscode",
      pid: process.pid,
    };
    const jsonPath = path.join(sessionsDir, `${sessionId}.json`);
    fs.writeFileSync(jsonPath, JSON.stringify(descriptor, null, 2) + "\n", "utf-8");

    // Write .env file for CLI tools
    const envPath = path.join(sessionsDir, `${sessionId}.env`);
    const envLines = [
      `GT_SESSION=${sessionId}`,
      `GT_RUN=${sessionId}`,
      `GT_ROLE=polecat`,
      `GT_RIG=${rig}`,
      `GT_AGENT=ghcp`,
      `GT_HEADLESS=true`,
      `GT_WORK_DIR=${descriptor.work_dir}`,
    ].join("\n") + "\n";
    fs.writeFileSync(envPath, envLines, "utf-8");

    log.appendLine(`Registered headless session at ${jsonPath}`);
  } catch (err) {
    log.appendLine(`Warning: could not register headless session: ${err}`);
  }
}

/** Detect the rig name from the current workspace. */
function detectRigFromWorkspace(): string {
  // Check GT_RIG env var first
  if (process.env.GT_RIG) return process.env.GT_RIG;
  // Fallback: workspace folder name
  const folder = vscode.workspace.workspaceFolders?.[0];
  if (folder) return path.basename(folder.uri.fsPath);
  return "unknown";
}

/** Emit session_start event to .events.jsonl */
function emitSessionStart(townRoot: string, sessionId: string): void {
  try {
    const eventsPath = path.join(townRoot, ".events.jsonl");
    const dir = path.dirname(eventsPath);
    if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
    const event = {
      ts: new Date().toISOString(),
      type: "session_start",
      actor: `${detectRigFromWorkspace()}/copilot`,
      payload: {
        session_id: sessionId,
        topic: "vscode-activate",
        agent: "github-copilot",
        platform: "vscode",
        headless: true,
      },
    };
    fs.appendFileSync(eventsPath, JSON.stringify(event) + "\n", "utf-8");
  } catch {
    // Silently ignore — Gas Town may not be installed
  }
}
