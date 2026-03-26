/**
 * Gas Town Convoy Dashboard — MCP App UI
 *
 * Connects to the ConvoyMCPapp server via the MCP Apps SDK and polls
 * the `get-gastown-data` tool every 10 seconds for live state.
 * Action buttons call `gt-sling` and `bd-close` on demand.
 */
import { App } from "@modelcontextprotocol/ext-apps";

// ── Types matching server.ts GastownData ─────────────────────────────────────

interface ReadyIssue {
  id: string;
  title: string;
  priority: number;
  type: string;
  status: string;
}

interface ConvoyRow {
  id: string;
  name: string;
  status: string;
  total: number;
  done: number;
  inProgress: number;
}

interface AgentRow {
  name: string;
  rig: string;
  status: string;
  currentIssue?: string;
}

interface GastownData {
  installed: { gt: boolean; bd: boolean };
  readyIssues: ReadyIssue[];
  convoys: ConvoyRow[];
  agents: AgentRow[];
  fetchedAt: string;
  errors: string[];
}

// ── DOM helpers ───────────────────────────────────────────────────────────────

function el<T extends HTMLElement>(id: string): T {
  return document.getElementById(id) as T;
}

function setText(id: string, text: string): void {
  const e = document.getElementById(id);
  if (e) e.textContent = text;
}

// ── Priority badge ─────────────────────────────────────────────────────────

function priBadge(p: number): string {
  const cls = p === 0 ? "p0" : p === 1 ? "p1" : p === 2 ? "p2" : "p3";
  return `<span class="row-badge ${cls}">P${p}</span>`;
}

function statusBadge(s: string): string {
  const lower = s.toLowerCase();
  const cls = lower.includes("progress") ? "status-progress"
    : lower === "done" || lower === "closed" ? "status-done"
    : "status-open";
  return `<span class="row-badge ${cls}">${s}</span>`;
}

// ── Render functions ──────────────────────────────────────────────────────────

function renderReadyWork(issues: ReadyIssue[]): void {
  const body = el<HTMLDivElement>("ready-body");
  setText("ready-count", String(issues.length));

  if (issues.length === 0) {
    body.innerHTML = `<div class="empty"><span class="empty-icon">✓</span>No unblocked issues</div>`;
    return;
  }

  body.innerHTML = issues
    .map(
      (i) =>
        `<div class="row" data-id="${esc(i.id)}" title="${esc(i.title)}">
          <span class="row-id">${esc(i.id)}</span>
          <span class="row-title">${esc(i.title)}</span>
          ${priBadge(i.priority)}
          <span class="row-meta">${esc(i.type)}</span>
        </div>`,
    )
    .join("");

  // Click to pre-fill action panel
  body.querySelectorAll<HTMLDivElement>(".row").forEach((row) => {
    row.addEventListener("click", () => {
      const id = row.dataset.id ?? "";
      selectIssue(id, row);
    });
  });
}

function selectIssue(id: string, row?: Element): void {
  el<HTMLInputElement>("input-issue").value = id;
  updateButtonState();

  // Highlight
  document.querySelectorAll(".row").forEach((r) => r.classList.remove("selected"));
  row?.classList.add("selected");
}

function renderConvoys(convoys: ConvoyRow[]): void {
  const body = el<HTMLDivElement>("convoy-body");
  setText("convoy-count", String(convoys.length));

  if (convoys.length === 0) {
    body.innerHTML = `<div class="empty"><span class="empty-icon">🏜️</span>No active convoys</div>`;
    return;
  }

  body.innerHTML = convoys
    .map(
      (c) =>
        `<div class="row">
          <span class="row-id">${esc(c.id)}</span>
          <span class="row-title">${esc(c.name)}</span>
          <span class="row-meta">${c.done}/${c.total}</span>
          ${statusBadge(c.status)}
        </div>`,
    )
    .join("");
}

function renderAgents(agents: AgentRow[]): void {
  const body = el<HTMLDivElement>("agent-body");
  setText("agent-count", String(agents.length));

  if (agents.length === 0) {
    body.innerHTML = `<div class="empty"><span class="empty-icon">💤</span>No agents running</div>`;
    return;
  }

  body.innerHTML = agents
    .map(
      (a) =>
        `<div class="row">
          <span class="row-id">${esc(a.name)}</span>
          <span class="row-title">${esc(a.rig)}${a.currentIssue ? ` → ${esc(a.currentIssue)}` : ""}</span>
          ${statusBadge(a.status)}
        </div>`,
    )
    .join("");
}

function renderAll(data: GastownData): void {
  const installed = data.installed.gt || data.installed.bd;
  el<HTMLDivElement>("not-installed").style.display = installed ? "none" : "block";
  el<HTMLDivElement>("main-grid").style.display = installed ? "grid" : "none";

  if (!installed) return;

  renderReadyWork(data.readyIssues);
  renderConvoys(data.convoys);
  renderAgents(data.agents);

  const hasErrors = data.errors.length > 0;
  const dot = el<HTMLDivElement>("status-dot");
  dot.className = "dot" + (hasErrors ? " warn" : "");
  setText("status-text", `Updated ${new Date(data.fetchedAt).toLocaleTimeString()}${hasErrors ? ` (${data.errors.length} warn)` : ""}`);
}

function esc(s: string | undefined | null): string {
  if (s == null) return "";
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

// ── App & polling ─────────────────────────────────────────────────────────────

const POLL_MS = 10_000;
let pollTimer: ReturnType<typeof setInterval> | null = null;

const app = new App(
  { name: "Gas Town Convoy Dashboard", version: "1.0.0" },
  { tools: { listChanged: true } },
  { autoResize: false },
);

app.onerror = console.error.bind(console, "[CONVOY]");
app.onteardown = async () => {
  if (pollTimer) clearInterval(pollTimer);
  return {};
};

app.ontoolinput = async () => {
  hideLoading();
  startPolling();
};

app.ontoolresult = async () => {
  hideLoading();
  startPolling();
};

async function initialize(): Promise<void> {
  try {
    await app.connect();
    console.log("[CONVOY] Connected to host");
    app.sendSizeChanged({ height: 600 });

    // Start polling after a short delay to let the bridge settle
    setTimeout(() => {
      hideLoading();
      startPolling();
    }, 500);
  } catch (error) {
    console.error("[CONVOY] Failed to connect:", error);
    hideLoading();
    setText("status-text", `Connection error: ${error instanceof Error ? error.message : String(error)}`);
    el<HTMLDivElement>("status-dot").className = "dot error";
  }
}

initialize();

function hideLoading(): void {
  el<HTMLDivElement>("loading").style.display = "none";
  el<HTMLDivElement>("app").style.display = "flex";
}

async function fetchData(): Promise<void> {
  try {
    const result = await app.callServerTool({ name: "get-gastown-data", arguments: {} });
    // Handle both direct content array and nested result.content patterns
    const content = result?.content ?? (result as unknown as { result: { content: unknown[] } })?.result?.content;
    const text = Array.isArray(content)
      ? content.find((c: { type: string }) => c.type === "text")
      : undefined;
    if (text && "text" in text) {
      const data = JSON.parse(text.text as string) as GastownData;
      renderAll(data);
    } else {
      // Try parsing result directly if it's the data object
      const raw = typeof result === "string" ? result : JSON.stringify(result);
      console.warn("[CONVOY] Unexpected result shape:", raw.substring(0, 200));
      setText("status-text", "Unexpected response format");
      el<HTMLDivElement>("status-dot").className = "dot warn";
    }
  } catch (e) {
    console.error("[CONVOY] fetch failed:", e);
    setText("status-text", `Error: ${String(e).substring(0, 60)}`);
    el<HTMLDivElement>("status-dot").className = "dot error";
  }
}

function startPolling(): void {
  if (pollTimer) return;
  void fetchData();
  pollTimer = setInterval(() => { void fetchData(); }, POLL_MS);
}

// ── Action buttons ─────────────────────────────────────────────────────────────

const inputIssue = el<HTMLInputElement>("input-issue");
const inputRig   = el<HTMLSelectElement>("input-rig");
const btnSling   = el<HTMLButtonElement>("btn-sling");
const btnClose   = el<HTMLButtonElement>("btn-close");
const btnRefresh = el<HTMLButtonElement>("btn-refresh");
const resultEl   = el<HTMLDivElement>("action-result");

function updateButtonState(): void {
  const issueVal = inputIssue.value.trim();
  const rigVal   = inputRig.value;
  btnSling.disabled = !issueVal || !rigVal;
  btnClose.disabled = !issueVal;
}

inputIssue.addEventListener("input", updateButtonState);
inputRig.addEventListener("change", updateButtonState);

function showResult(msg: string, ok: boolean): void {
  resultEl.textContent = msg;
  resultEl.className = ok ? "ok" : "err";
}

function clearResult(): void {
  resultEl.textContent = "";
  resultEl.className = "";
}

btnSling.addEventListener("click", async () => {
  const issueId = inputIssue.value.trim();
  const rig = inputRig.value;
  if (!issueId || !rig) return;

  btnSling.disabled = true;
  clearResult();
  try {
    const result = await app.callServerTool({
      name: "gt-sling",
      arguments: { issueId, rig },
    });
    const text = result.content?.find((c: { type: string }) => c.type === "text");
    const msg = text && "text" in text ? String(text.text) : "Done";
    showResult(msg, !result.isError);
    if (!result.isError) void fetchData();
  } catch (e) {
    showResult(String(e), false);
  } finally {
    updateButtonState();
  }
});

btnClose.addEventListener("click", async () => {
  const issueId = inputIssue.value.trim();
  if (!issueId) return;

  const reason = prompt(`Close ${issueId} — reason (optional):`);
  if (reason === null) return; // cancelled

  btnClose.disabled = true;
  clearResult();
  try {
    const result = await app.callServerTool({
      name: "bd-close",
      arguments: { issueId, ...(reason ? { reason } : {}) },
    });
    const text = result.content?.find((c: { type: string }) => c.type === "text");
    const msg = text && "text" in text ? String(text.text) : "Done";
    showResult(msg, !result.isError);
    if (!result.isError) {
      inputIssue.value = "";
      updateButtonState();
      void fetchData();
    }
  } catch (e) {
    showResult(String(e), false);
  } finally {
    updateButtonState();
  }
});

btnRefresh.addEventListener("click", () => {
  clearResult();
  void fetchData();
});
