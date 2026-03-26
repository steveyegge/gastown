/**
 * Heartbeat Emitter
 *
 * Periodically writes a heartbeat file to signal that this headless
 * GHCP session is alive. Gas Town's daemon and witness read these
 * files to determine session liveness (replacing tmux.HasSession).
 *
 * Heartbeat path: <townRoot>/.runtime/heartbeat/<sessionId>.json
 * Frequency: every 60 seconds
 *
 * Also cleans up the heartbeat file on dispose
 * (VS Code window close / extension deactivation).
 */
import * as fs from "fs";
import * as path from "path";

/** Minimal Disposable interface (avoids hard vscode import). */
interface Disposable { dispose(): void; }

export class HeartbeatEmitter implements Disposable {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private timer: any;
  private heartbeatPath: string;
  private sessionDescPath: string;

  constructor(
    private townRoot: string,
    private sessionId: string,
  ) {
    const heartbeatDir = path.join(townRoot, ".runtime", "heartbeat");
    fs.mkdirSync(heartbeatDir, { recursive: true });
    this.heartbeatPath = path.join(heartbeatDir, `${sessionId}.json`);
    this.sessionDescPath = path.join(townRoot, ".runtime", "sessions", `${sessionId}.json`);

    // Write initial heartbeat
    this.write("active");

    // Write every 60 seconds
    this.timer = setInterval(() => this.write("active"), 60_000);
  }

  /** Update status (e.g. "busy" while processing, "idle" when waiting). */
  setStatus(status: "active" | "idle" | "busy"): void {
    this.write(status);
  }

  private write(status: string): void {
    try {
      const heartbeat = {
        session_id: this.sessionId,
        timestamp: new Date().toISOString(),
        status,
        platform: "vscode-copilot",
      };
      fs.writeFileSync(this.heartbeatPath, JSON.stringify(heartbeat) + "\n", "utf-8");
    } catch {
      // Silently ignore — Gas Town may not be installed
    }
  }

  dispose(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
    // Clean up heartbeat file on shutdown (session is ending)
    try { fs.unlinkSync(this.heartbeatPath); } catch { /* ignore */ }
  }
}
