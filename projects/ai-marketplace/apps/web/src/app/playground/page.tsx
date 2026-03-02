"use client";

import { useState } from "react";
import { PanelLeftClose, PanelLeftOpen, Sliders, ChevronRight } from "lucide-react";
import { ChatPane } from "@/components/playground/ChatPane";
import { PreviewPane } from "@/components/playground/PreviewPane";
import { ConfigPanel } from "@/components/playground/ConfigPanel";
import { usePlaygroundStore } from "@/lib/stores/playgroundStore";
import Link from "next/link";

export default function PlaygroundPage() {
  const [configOpen, setConfigOpen] = useState(true);
  const [chatWidth, setChatWidth] = useState(420); // px
  const config = usePlaygroundStore((s) => s.config);
  const messages = usePlaygroundStore((s) => s.messages);
  const isStreaming = usePlaygroundStore((s) => s.isStreaming);

  return (
    <div className="-mx-6 -my-6 flex h-full flex-col overflow-hidden">
      {/* ── Top bar ───────────────────────────────────────────────────── */}
      <header className="flex h-12 flex-shrink-0 items-center justify-between border-b bg-white px-4">
        {/* Left: breadcrumb */}
        <div className="flex items-center gap-1.5 text-sm">
          <Link href="/" className="text-gray-400 hover:text-gray-600 transition-colors">AI Asset Marketplace</Link>
          <ChevronRight className="h-3.5 w-3.5 text-gray-300" />
          <span className="font-semibold text-gray-800">Playground</span>
        </div>

        {/* Center: agent + model pill */}
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-violet-100 px-3 py-0.5 text-xs font-medium text-violet-700">
            {config.agentName === "(No agent — direct model)" ? "Direct" : config.agentName}
          </span>
          <span className="rounded-full bg-blue-100 px-3 py-0.5 text-xs font-medium text-blue-700">
            {config.model}
          </span>
          {isStreaming && (
            <span className="flex items-center gap-1 rounded-full bg-green-100 px-2.5 py-0.5 text-[11px] font-medium text-green-700">
              <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse" />
              Streaming
            </span>
          )}
        </div>

        {/* Right: toggle config panel */}
        <button
          onClick={() => setConfigOpen((v) => !v)}
          className="flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 transition-colors"
        >
          <Sliders className="h-3.5 w-3.5" />
          Config
          {configOpen ? <PanelLeftClose className="h-3.5 w-3.5" /> : <PanelLeftOpen className="h-3.5 w-3.5" />}
        </button>
      </header>

      {/* ── Main 3-column layout ──────────────────────────────────────── */}
      <div className="flex flex-1 overflow-hidden">

        {/* Config panel (left, collapsible) */}
        {configOpen && (
          <aside className="w-60 flex-shrink-0 border-r overflow-hidden">
            <ConfigPanel onClose={() => setConfigOpen(false)} />
          </aside>
        )}

        {/* Chat pane (center-left) */}
        <div
          className="flex-shrink-0 border-r bg-gray-50 flex flex-col overflow-hidden"
          style={{ width: configOpen ? "min(38%, 440px)" : "min(42%, 500px)" }}
        >
          {/* Pane header */}
          <div className="flex h-10 items-center justify-between border-b bg-white px-4">
            <span className="text-xs font-semibold text-gray-500 uppercase tracking-wide">Chat</span>
            <span className="text-[10px] text-gray-400">
              {messages.length} message{messages.length !== 1 ? "s" : ""}
            </span>
          </div>
          <div className="flex-1 overflow-hidden">
            <ChatPane />
          </div>
        </div>

        {/* Preview pane (right, takes remaining space) */}
        <div className="flex flex-1 flex-col overflow-hidden">
          <PreviewPane />
        </div>
      </div>
    </div>
  );
}
