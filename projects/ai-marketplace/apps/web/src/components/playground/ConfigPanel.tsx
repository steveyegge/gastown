"use client";

import { Settings2, ChevronDown, Bot, Sliders, X } from "lucide-react";
import { useState } from "react";
import { usePlaygroundStore, MODELS, SAMPLE_AGENTS } from "@/lib/stores/playgroundStore";

function Slider({ label, value, min, max, step, onChange, format }: {
  label: string; value: number; min: number; max: number; step: number;
  onChange: (v: number) => void; format?: (v: number) => string;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex justify-between text-xs">
        <span className="text-gray-600">{label}</span>
        <span className="font-mono font-semibold text-gray-800">{format ? format(value) : value}</span>
      </div>
      <input
        type="range" min={min} max={max} step={step} value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        className="w-full h-1.5 accent-blue-600 cursor-pointer"
      />
      <div className="flex justify-between text-[10px] text-gray-400">
        <span>{min}</span><span>{max}</span>
      </div>
    </div>
  );
}

function Select<T extends string>({ label, value, options, onChange }: {
  label: string;
  value: T;
  options: { value: T; label: string; sub?: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium text-gray-600">{label}</label>
      <div className="relative">
        <select
          value={value}
          onChange={(e) => onChange(e.target.value as T)}
          className="w-full appearance-none rounded-lg border border-gray-200 bg-white px-3 py-2 pr-8 text-sm text-gray-800 shadow-sm focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-400/20"
        >
          {options.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
        <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
      </div>
    </div>
  );
}

export function ConfigPanel({ onClose }: { onClose?: () => void }) {
  const config = usePlaygroundStore((s) => s.config);
  const setConfig = usePlaygroundStore((s) => s.setConfig);
  const [showSystem, setShowSystem] = useState(false);

  const modelOpts = MODELS.map((m) => ({ value: m.id, label: `${m.label}`, sub: m.provider }));
  const agentOpts = SAMPLE_AGENTS.map((a) => ({ value: a.id ?? "__none__", label: a.name }));

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold text-gray-800">
          <Sliders className="h-4 w-4 text-blue-500" />
          Configuration
        </div>
        {onClose && (
          <button onClick={onClose} className="rounded p-1 hover:bg-gray-100">
            <X className="h-4 w-4 text-gray-400" />
          </button>
        )}
      </div>

      <div className="flex-1 space-y-5 px-4 py-4">
        {/* Agent */}
        <div>
          <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-gray-500">
            <Bot className="h-3.5 w-3.5" /> Agent
          </div>
          <Select
            label=""
            value={config.agentId ?? "__none__"}
            options={agentOpts}
            onChange={(v) => setConfig({ agentId: v === "__none__" ? null : v, agentName: SAMPLE_AGENTS.find((a) => (a.id ?? "__none__") === v)?.name ?? "" })}
          />
        </div>

        {/* Model */}
        <Select
          label="Model"
          value={config.model}
          options={modelOpts}
          onChange={(v) => setConfig({ model: v })}
        />

        {/* Sliders */}
        <div className="space-y-4 rounded-xl bg-gray-50 p-4">
          <Slider
            label="Temperature"
            value={config.temperature}
            min={0} max={2} step={0.05}
            onChange={(v) => setConfig({ temperature: v })}
            format={(v) => v.toFixed(2)}
          />
          <Slider
            label="Max tokens"
            value={config.maxTokens}
            min={256} max={8192} step={256}
            onChange={(v) => setConfig({ maxTokens: v })}
            format={(v) => v.toLocaleString()}
          />
          <Slider
            label="Top P"
            value={config.topP}
            min={0} max={1} step={0.01}
            onChange={(v) => setConfig({ topP: v })}
            format={(v) => v.toFixed(2)}
          />
        </div>

        {/* Stream toggle */}
        <label className="flex items-center justify-between cursor-pointer">
          <span className="text-sm text-gray-700">Stream response</span>
          <div className="relative">
            <input
              type="checkbox"
              checked={config.stream}
              onChange={(e) => setConfig({ stream: e.target.checked })}
              className="sr-only"
            />
            <div className={`h-5 w-9 rounded-full transition-colors ${config.stream ? "bg-blue-600" : "bg-gray-300"}`}>
              <div className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${config.stream ? "translate-x-4" : "translate-x-0.5"}`} />
            </div>
          </div>
        </label>

        {/* System prompt */}
        <div className="space-y-1.5">
          <button
            onClick={() => setShowSystem((v) => !v)}
            className="flex w-full items-center justify-between text-xs font-medium text-gray-600 hover:text-gray-900"
          >
            <span>System prompt</span>
            <Settings2 className="h-3.5 w-3.5" />
          </button>
          {showSystem && (
            <textarea
              rows={5}
              value={config.systemPrompt}
              onChange={(e) => setConfig({ systemPrompt: e.target.value })}
              className="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs text-gray-700 focus:border-blue-400 focus:outline-none focus:ring-2 focus:ring-blue-400/20 resize-none"
            />
          )}
        </div>

        {/* Quick prompts */}
        <div className="space-y-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">Suggested prompts</p>
          {[
            "Summarize the denial intelligence workflow",
            "Show me a TypeScript SDK snippet",
            "Which agents handle PII data safely?",
          ].map((prompt) => (
            <button
              key={prompt}
              onClick={() => {
                // Inject prompt into the chat input – dispatch custom event
                window.dispatchEvent(new CustomEvent("playground:set-draft", { detail: prompt }));
              }}
              className="w-full rounded-lg border border-dashed border-gray-200 px-3 py-2 text-left text-xs text-gray-600 hover:border-blue-300 hover:bg-blue-50 hover:text-blue-700 transition-colors"
            >
              {prompt}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
