"use client";

import { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Bot, User, Copy, Check, StopCircle, Loader2 } from "lucide-react";
import {
  usePlaygroundStore,
  sendMessage,
  type ChatMessage,
} from "@/lib/stores/playgroundStore";

// ─── Single message bubble ───────────────────────────────────────────────────

function AssistantBubble({ content, meta, isCurrent }: {
  content: string;
  meta?: ChatMessage["meta"];
  isCurrent?: boolean;
}) {
  return (
    <div className="group flex gap-3">
      <div className="mt-0.5 h-7 w-7 flex-shrink-0 rounded-full bg-gradient-to-br from-blue-500 to-violet-600 flex items-center justify-center">
        <Bot className="h-3.5 w-3.5 text-white" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="prose prose-sm max-w-none text-gray-800 [&_table]:block [&_table]:overflow-x-auto [&_pre]:bg-gray-900 [&_pre]:rounded-lg [&_pre]:p-3 [&_code]:text-sm [&_pre_code]:text-gray-100 [&_code:not(pre_code)]:bg-gray-100 [&_code:not(pre_code)]:px-1 [&_code:not(pre_code)]:rounded [&_code:not(pre_code)]:text-gray-800">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          {isCurrent && (
            <span className="inline-block h-4 w-0.5 animate-pulse bg-blue-500 ml-0.5 -mb-0.5" />
          )}
        </div>
        {meta && (
          <p className="mt-1.5 text-[10px] text-gray-400">
            {meta.model} · {meta.latencyMs}ms · {meta.finishReason}
          </p>
        )}
      </div>
    </div>
  );
}

function UserBubble({ content }: { content: string }) {
  return (
    <div className="flex gap-3 justify-end">
      <div className="max-w-[80%]">
        <div className="rounded-2xl rounded-tr-sm bg-blue-600 px-4 py-2.5 text-sm text-white whitespace-pre-wrap break-words">
          {content}
        </div>
      </div>
      <div className="mt-0.5 h-7 w-7 flex-shrink-0 rounded-full bg-gray-200 flex items-center justify-center">
        <User className="h-3.5 w-3.5 text-gray-600" />
      </div>
    </div>
  );
}

// ─── Code copy button ────────────────────────────────────────────────────────

export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 2000); }}
      className="p-1 rounded hover:bg-white/10 transition-colors"
    >
      {copied ? <Check className="h-3.5 w-3.5 text-green-400" /> : <Copy className="h-3.5 w-3.5 text-gray-400" />}
    </button>
  );
}

// ─── Main chat pane ──────────────────────────────────────────────────────────

export function ChatPane() {
  const messages = usePlaygroundStore((s) => s.messages);
  const isStreaming = usePlaygroundStore((s) => s.isStreaming);
  const currentOutput = usePlaygroundStore((s) => s.currentOutput);
  const clearConversation = usePlaygroundStore((s) => s.clearConversation);
  const tokenCount = usePlaygroundStore((s) => s.tokenCount);

  const [draft, setDraft] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Listen for suggested prompt events from ConfigPanel
  useEffect(() => {
    function handler(e: Event) {
      setDraft((e as CustomEvent<string>).detail);
      textareaRef.current?.focus();
    }
    window.addEventListener("playground:set-draft", handler);
    return () => window.removeEventListener("playground:set-draft", handler);
  }, []);

  // Auto-scroll on new content
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages.length, currentOutput]);

  // Auto-grow textarea
  useEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 160) + "px";
  }, [draft]);

  async function handleSend() {
    const text = draft.trim();
    if (!text || isStreaming) return;
    setDraft("");
    await sendMessage(text);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-5">
        {messages.length === 0 && !isStreaming && (
          <div className="flex h-full flex-col items-center justify-center text-center gap-3 opacity-60 pt-16">
            <Bot className="h-10 w-10 text-blue-400" />
            <p className="text-sm font-medium text-gray-600">Start with a prompt</p>
            <p className="text-xs text-gray-400 max-w-[220px]">
              Ask about agents, get code snippets, or run a denial analysis.
            </p>
          </div>
        )}
        {messages.map((msg) =>
          msg.role === "user" ? (
            <UserBubble key={msg.id} content={msg.content} />
          ) : (
            <AssistantBubble key={msg.id} content={msg.content} meta={msg.meta} />
          )
        )}
        {isStreaming && currentOutput && (
          <AssistantBubble content={currentOutput} isCurrent />
        )}
        {isStreaming && !currentOutput && (
          <div className="flex gap-3">
            <div className="mt-0.5 h-7 w-7 flex-shrink-0 rounded-full bg-gradient-to-br from-blue-500 to-violet-600 flex items-center justify-center">
              <Bot className="h-3.5 w-3.5 text-white" />
            </div>
            <div className="flex items-center gap-1 pt-2">
              {[0,1,2].map(i => (
                <span key={i} className="h-1.5 w-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: `${i * 0.12}s` }} />
              ))}
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Token counter */}
      {tokenCount > 0 && (
        <div className="px-4 pb-1 text-right">
          <span className="text-[10px] text-gray-400">~{tokenCount.toLocaleString()} tokens used</span>
        </div>
      )}

      {/* Input */}
      <div className="border-t bg-white px-4 py-3">
        <div className="flex items-end gap-2 rounded-xl border bg-gray-50 px-3 py-2.5 focus-within:border-blue-400 focus-within:ring-2 focus-within:ring-blue-400/20 transition-all">
          <textarea
            ref={textareaRef}
            rows={1}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Message the agent… (⏎ to send, Shift+⏎ for newline)"
            className="flex-1 resize-none bg-transparent text-sm text-gray-800 placeholder:text-gray-400 focus:outline-none"
            disabled={isStreaming}
          />
          <div className="flex items-center gap-1.5 self-end">
            {messages.length > 0 && !isStreaming && (
              <button
                onClick={clearConversation}
                className="rounded-lg px-2 py-1.5 text-xs text-gray-400 hover:text-red-500 hover:bg-red-50 transition-colors"
              >
                Clear
              </button>
            )}
            {isStreaming ? (
              <button className="flex items-center gap-1 rounded-lg bg-red-100 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-200 transition-colors">
                <StopCircle className="h-3.5 w-3.5" /> Stop
              </button>
            ) : (
              <button
                onClick={handleSend}
                disabled={!draft.trim()}
                className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-semibold text-white shadow-sm hover:bg-blue-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                <Loader2 className={`h-3 w-3 ${isStreaming ? "animate-spin" : "hidden"}`} />
                Send
              </button>
            )}
          </div>
        </div>
        <p className="mt-1.5 text-center text-[10px] text-gray-400">
          AI-generated content may be inaccurate. Review before use in production.
        </p>
      </div>
    </div>
  );
}
