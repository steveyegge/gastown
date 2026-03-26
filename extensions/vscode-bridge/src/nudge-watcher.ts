/**
 * Nudge Watcher
 *
 * Watches the Gas Town nudge queue directory for incoming nudges
 * and surfaces them as VS Code notifications. This replaces the
 * tmux send-keys delivery mechanism for headless GHCP sessions.
 *
 * Queue path: <townRoot>/.runtime/nudge_queue/<sessionId>/
 * Nudge format: JSON files with sender, message, priority, kind.
 *
 * Uses VS Code's native FileSystemWatcher (inotify/FSEvents/ReadDirectoryChanges)
 * for zero-latency delivery — no polling needed.
 */
import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";

interface QueuedNudge {
  sender: string;
  message: string;
  priority: string;
  kind?: string;
  thread_id?: string;
  severity?: string;
  timestamp: string;
  expires_at?: string;
}

export class NudgeWatcher implements vscode.Disposable {
  private disposables: vscode.Disposable[] = [];
  private queueDir: string;
  private watcher: vscode.FileSystemWatcher | undefined;
  private outputChannel: vscode.OutputChannel;

  constructor(
    private townRoot: string,
    private sessionId: string,
  ) {
    this.queueDir = path.join(townRoot, ".runtime", "nudge_queue", sessionId);
    this.outputChannel = vscode.window.createOutputChannel("Gas Town Nudges");

    // Ensure queue directory exists so the watcher can attach
    fs.mkdirSync(this.queueDir, { recursive: true });

    // Watch for new nudge files
    const pattern = new vscode.RelativePattern(this.queueDir, "*.json");
    this.watcher = vscode.workspace.createFileSystemWatcher(pattern);

    this.watcher.onDidCreate((uri: vscode.Uri) => this.onNudgeFile(uri));
    // Also handle renames (some producers write then rename)
    this.watcher.onDidChange((uri: vscode.Uri) => this.onNudgeFile(uri));

    this.disposables.push(this.watcher, this.outputChannel);

    // Drain any nudges that arrived before we started watching
    this.drainExisting();
  }

  private async drainExisting(): Promise<void> {
    try {
      const files = fs.readdirSync(this.queueDir)
        .filter((f: string) => f.endsWith(".json"))
        .sort();
      for (const f of files) {
        await this.processNudgeFile(path.join(this.queueDir, f));
      }
    } catch {
      // Queue dir may not exist yet
    }
  }

  private async onNudgeFile(uri: vscode.Uri): Promise<void> {
    if (!uri.fsPath.endsWith(".json")) return;
    // Small delay to let the writer finish
    await new Promise<void>(r => setTimeout(r, 50));
    await this.processNudgeFile(uri.fsPath);
  }

  private async processNudgeFile(filePath: string): Promise<void> {
    try {
      const raw = fs.readFileSync(filePath, "utf-8");
      const nudge: QueuedNudge = JSON.parse(raw);

      // Check expiry
      if (nudge.expires_at && new Date(nudge.expires_at) < new Date()) {
        fs.unlinkSync(filePath);
        return;
      }

      // Claim: rename to .claimed to prevent double delivery
      const claimedPath = filePath.replace(".json", `.claimed.${Date.now()}`);
      try {
        fs.renameSync(filePath, claimedPath);
      } catch {
        return; // Another consumer claimed it
      }

      // Surface the nudge
      this.surfaceNudge(nudge);

      // Clean up claimed file
      try { fs.unlinkSync(claimedPath); } catch { /* ignore */ }
    } catch {
      // Corrupt or already consumed
    }
  }

  private surfaceNudge(nudge: QueuedNudge): void {
    const prefix = nudge.priority === "urgent" ? "🚨" : "📬";
    const label = `${prefix} [${nudge.sender}] ${nudge.message}`;

    // Log to output channel
    this.outputChannel.appendLine(`[${new Date().toISOString()}] ${label}`);

    // Show VS Code notification
    if (nudge.priority === "urgent" || nudge.severity === "critical") {
      vscode.window.showWarningMessage(label, "View Nudges").then((action: string | undefined) => {
        if (action === "View Nudges") this.outputChannel.show();
      });
    } else {
      vscode.window.showInformationMessage(label);
    }
  }

  dispose(): void {
    for (const d of this.disposables) d.dispose();
    this.disposables = [];
  }
}
