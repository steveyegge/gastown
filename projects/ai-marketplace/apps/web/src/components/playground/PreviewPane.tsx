"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Copy, Check, FileCode, ScrollText, Eye } from "lucide-react";
import { usePlaygroundStore } from "@/lib/stores/playgroundStore";

// ─── Extract code blocks from markdown ───────────────────────────────────────

interface CodeBlock {
  lang: string;
  code: string;
}

function extractCodeBlocks(markdown: string): CodeBlock[] {
  const re = /```(\w*)\n([\s\S]*?)```/g;
  const blocks: CodeBlock[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(markdown)) !== null) {
    blocks.push({ lang: m[1] || "text", code: m[2].trimEnd() });
  }
  return blocks;
}

// ─── Copy button ─────────────────────────────────────────────────────────────

function CopyBtn({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 2000); }}
      className="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-gray-400 hover:text-gray-200 hover:bg-white/10 transition-colors"
    >
      {copied ? <Check className="h-3.5 w-3.5 text-green-400" /> : <Copy className="h-3.5 w-3.5" />}
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

// ─── Preview tab ─────────────────────────────────────────────────────────────

function PreviewTab({ content }: { content: string }) {
  if (!content) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-center gap-3 opacity-40">
        <Eye className="h-10 w-10 text-gray-400" />
        <p className="text-sm text-gray-500">Output will appear here</p>
      </div>
    );
  }
  return (
    <div className="h-full overflow-y-auto px-8 py-6">
      <article className="prose prose-sm max-w-3xl mx-auto text-gray-800
        [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:mt-6 [&_h2]:mb-3
        [&_h3]:font-semibold [&_h3]:mt-4 [&_h3]:mb-2
        [&_pre]:bg-gray-900 [&_pre]:rounded-xl [&_pre]:p-4 [&_pre]:overflow-x-auto
        [&_pre_code]:text-sm [&_pre_code]:text-gray-100 [&_pre_code]:font-mono
        [&_code:not(pre_code)]:bg-blue-50 [&_code:not(pre_code)]:text-blue-700 [&_code:not(pre_code)]:px-1.5 [&_code:not(pre_code)]:py-0.5 [&_code:not(pre_code)]:rounded [&_code:not(pre_code)]:text-xs
        [&_table]:w-full [&_table]:border-collapse
        [&_th]:bg-gray-100 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:text-xs [&_th]:font-semibold [&_th]:text-gray-600 [&_th]:border [&_th]:border-gray-200
        [&_td]:px-3 [&_td]:py-2 [&_td]:text-sm [&_td]:border [&_td]:border-gray-200
        [&_blockquote]:border-l-4 [&_blockquote]:border-blue-400 [&_blockquote]:bg-blue-50 [&_blockquote]:px-4 [&_blockquote]:py-3 [&_blockquote]:rounded-r-lg [&_blockquote]:not-italic
        [&_ul]:space-y-0.5 [&_ol]:space-y-0.5">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
      </article>
    </div>
  );
}

// ─── Code tab ────────────────────────────────────────────────────────────────

function CodeTab({ content }: { content: string }) {
  const blocks = extractCodeBlocks(content);
  if (!blocks.length) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-center gap-3 opacity-40">
        <FileCode className="h-10 w-10 text-gray-400" />
        <p className="text-sm text-gray-500">No code blocks in the last response</p>
      </div>
    );
  }
  return (
    <div className="h-full overflow-y-auto px-6 py-5 space-y-5">
      {blocks.map((b, i) => (
        <div key={i} className="rounded-xl overflow-hidden border border-gray-200">
          <div className="flex items-center justify-between bg-gray-800 px-4 py-2">
            <span className="text-xs font-mono text-gray-400">{b.lang || "text"}</span>
            <CopyBtn text={b.code} />
          </div>
          <pre className="bg-gray-900 px-4 py-4 overflow-x-auto">
            <code className="text-sm font-mono text-gray-100 leading-relaxed">{b.code}</code>
          </pre>
        </div>
      ))}
    </div>
  );
}

// ─── Logs tab ────────────────────────────────────────────────────────────────

function LogsTab({ logs }: { logs: string[] }) {
  if (!logs.length) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-center gap-3 opacity-40">
        <ScrollText className="h-10 w-10 text-gray-400" />
        <p className="text-sm text-gray-500">Execution logs will appear here</p>
      </div>
    );
  }
  return (
    <div className="h-full overflow-y-auto px-4 py-4 font-mono text-xs">
      {logs.map((line, i) => {
        const isIn = line.includes("→");
        const isOut = line.includes("←");
        return (
          <div key={i} className={`py-0.5 ${isIn ? "text-blue-400" : isOut ? "text-green-400" : "text-gray-400"}`}>
            {line}
          </div>
        );
      })}
    </div>
  );
}

// ─── Preview pane ─────────────────────────────────────────────────────────────

type Tab = "preview" | "code" | "logs";

export function PreviewPane() {
  const messages = usePlaygroundStore((s) => s.messages);
  const currentOutput = usePlaygroundStore((s) => s.currentOutput);
  const logs = usePlaygroundStore((s) => s.logs);
  const isStreaming = usePlaygroundStore((s) => s.isStreaming);
  const [tab, setTab] = useState<Tab>("preview");

  // Latest assistant content (either finalised or streaming)
  const lastAssistant = [...messages].reverse().find((m) => m.role === "assistant");
  const previewContent = isStreaming ? currentOutput : (lastAssistant?.content ?? "");

  const codeBlocks = extractCodeBlocks(previewContent);

  const TABS: { id: Tab; label: string; badge?: number }[] = [
    { id: "preview", label: "Preview" },
    { id: "code",    label: "Code", badge: codeBlocks.length || undefined },
    { id: "logs",    label: "Logs", badge: logs.length || undefined },
  ];

  return (
    <div className="flex h-full flex-col bg-white">
      {/* Tab bar */}
      <div className="flex items-center gap-0.5 border-b bg-gray-50 px-4 py-0">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`relative flex items-center gap-1.5 px-4 py-3 text-sm font-medium transition-colors border-b-2 -mb-px ${
              tab === t.id
                ? "border-blue-600 text-blue-700"
                : "border-transparent text-gray-500 hover:text-gray-800"
            }`}
          >
            {t.label}
            {t.badge != null && t.badge > 0 && (
              <span className="rounded-full bg-gray-200 px-1.5 py-0.5 text-[10px] font-semibold text-gray-600">
                {t.badge}
              </span>
            )}
            {t.id === "preview" && isStreaming && (
              <span className="ml-1 h-1.5 w-1.5 rounded-full bg-blue-500 animate-pulse" />
            )}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden bg-white">
        {tab === "preview" && <PreviewTab content={previewContent} />}
        {tab === "code"    && <CodeTab content={previewContent} />}
        {tab === "logs"    && <LogsTab logs={logs} />}
      </div>
    </div>
  );
}
