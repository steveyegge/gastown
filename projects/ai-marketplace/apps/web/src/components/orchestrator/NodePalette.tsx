"use client";

const NODE_PALETTE_ITEMS = [
  {
    category: "Agents",
    items: [
      { type: "agent", label: "AI Agent", icon: "🤖", desc: "Autonomous AI agent from catalog" },
      { type: "agent", label: "Custom Agent", icon: "🤖", desc: "Custom-configured agent" },
    ],
  },
  {
    category: "MCP",
    items: [
      { type: "tool", label: "MCP Server", icon: "🔧", desc: "MCP server connection" },
      { type: "tool", label: "API Connector", icon: "🔌", desc: "OpenAPI connector" },
    ],
  },
  {
    category: "Knowledge",
    items: [
      { type: "knowledge", label: "Knowledge Base", icon: "📚", desc: "Vector search / RAG" },
      { type: "knowledge", label: "Document Store", icon: "🗂️", desc: "Azure AI Search grounding" },
    ],
  },
  {
    category: "Models",
    items: [
      { type: "model", label: "Azure OpenAI", icon: "🧠", desc: "GPT-4o, GPT-4 Turbo" },
      { type: "model", label: "Phi-4", icon: "🧠", desc: "Microsoft Phi-4 small model" },
    ],
  },
  {
    category: "Evaluation",
    items: [
      { type: "evaluator", label: "Evaluator", icon: "📊", desc: "Run evaluation / test suite" },
      { type: "evaluator", label: "Groundedness Check", icon: "✅", desc: "Verify output grounding" },
    ],
  },
  {
    category: "Control",
    items: [
      { type: "human", label: "Human Gate", icon: "👤", desc: "Require human approval" },
      { type: "guard", label: "Policy Guard", icon: "🛡️", desc: "Enforce content/data policy" },
    ],
  },
];

export function NodePalette() {
  const onDragStart = (event: React.DragEvent, type: string, label: string) => {
    event.dataTransfer.setData("application/reactflow/type", type);
    event.dataTransfer.setData("application/reactflow/label", label);
    event.dataTransfer.effectAllowed = "move";
  };

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <div className="border-b px-4 py-3">
        <p className="text-xs font-semibold uppercase tracking-wider text-gray-400">
          Node Palette
        </p>
        <p className="mt-0.5 text-xs text-gray-400">Drag to canvas</p>
      </div>
      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {NODE_PALETTE_ITEMS.map((group) => (
          <div key={group.category}>
            <p className="mb-2 text-xs font-semibold text-gray-500">{group.category}</p>
            <div className="space-y-1">
              {group.items.map((item) => (
                <div
                  key={`${item.type}-${item.label}`}
                  draggable
                  onDragStart={(e) => onDragStart(e, item.type, item.label)}
                  className="flex cursor-grab items-center gap-2.5 rounded-lg border bg-white p-2.5 text-xs shadow-sm transition-colors hover:border-blue-300 hover:bg-blue-50 active:cursor-grabbing"
                >
                  <span className="text-base">{item.icon}</span>
                  <div>
                    <p className="font-medium text-gray-800">{item.label}</p>
                    <p className="text-gray-400 text-[10px]">{item.desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
