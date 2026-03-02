import { create } from "zustand";

export type MessageRole = "user" | "assistant" | "system";

export interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  /** token count estimate */
  tokens?: number;
  /** only for assistant messages – execution metadata */
  meta?: {
    model: string;
    latencyMs: number;
    finishReason: "stop" | "length" | "error";
  };
}

export interface PlaygroundConfig {
  model: string;
  agentId: string | null;
  agentName: string;
  systemPrompt: string;
  temperature: number;
  maxTokens: number;
  topP: number;
  stream: boolean;
}

interface PlaygroundStore {
  config: PlaygroundConfig;
  messages: ChatMessage[];
  isStreaming: boolean;
  activeTab: "preview" | "code" | "logs";
  currentOutput: string;  // streamed output being built
  tokenCount: number;
  logs: string[];

  setConfig: (patch: Partial<PlaygroundConfig>) => void;
  addMessage: (msg: ChatMessage) => void;
  setMessages: (msgs: ChatMessage[]) => void;
  appendToOutput: (chunk: string) => void;
  finalizeOutput: (meta: ChatMessage["meta"]) => void;
  setStreaming: (v: boolean) => void;
  setActiveTab: (tab: PlaygroundStore["activeTab"]) => void;
  addLog: (line: string) => void;
  clearConversation: () => void;
}

const DEFAULT_SYSTEM_PROMPT =
  "You are a helpful AI assistant for the AI Asset Marketplace. Help users discover, evaluate, and orchestrate AI assets including agents, MCP servers, models, and workflow templates.";

export const MODELS = [
  { id: "gpt-4o",           label: "GPT-4o",           provider: "Azure OpenAI" },
  { id: "gpt-4o-mini",      label: "GPT-4o mini",       provider: "Azure OpenAI" },
  { id: "o1-preview",       label: "o1 preview",        provider: "Azure OpenAI" },
  { id: "o3-mini",          label: "o3-mini",           provider: "Azure OpenAI" },
  { id: "phi-4",            label: "Phi-4",             provider: "Azure AI" },
  { id: "mistral-large",    label: "Mistral Large",     provider: "Azure AI" },
  { id: "llama-3.3-70b",    label: "Llama 3.3 70B",     provider: "Azure AI" },
];

export const SAMPLE_AGENTS = [
  { id: null,           name: "(No agent — direct model)" },
  { id: "denial-agent", name: "Denial Intelligence Agent" },
  { id: "coder-agent",  name: "Code Generator Agent" },
  { id: "rag-agent",    name: "Clinical RAG Agent" },
];

// Canned streamed replies for demo mode (no real backend needed)
const CANNED: Record<string, string> = {
  default: `I'm running on **{model}** through the AI Asset Marketplace playground.\n\nI can help you:\n- 🔍 **Discover** agents, tools, and models in the catalog\n- 🧪 **Evaluate** assets against your compliance requirements\n- 🔗 **Orchestrate** multi-agent workflows on the canvas\n- 🚀 **Deploy** workflows to Azure AI Foundry environments\n\nWhat would you like to explore?`,
  deny: `## Denial Intelligence Analysis\n\nBased on the claim data provided, here's what I found:\n\n| Factor | Finding |\n|--------|--------|\n| **Primary denial reason** | Missing prior authorization (code 4N1) |\n| **Payer** | Medicare Advantage |\n| **Appeal window** | 60 days remaining |\n\n### Recommended action\n1. Submit PA form CMS-1500 with supporting clinical notes\n2. Attach prior records showing medical necessity\n3. Reference NCCI edit bundle 99213+99386\n\n> ⚠️ **Confidence**: 87% — reviewed against 14,200 historical claims`,
  code: `Here's a TypeScript snippet to call this agent via the MCP protocol:\n\n\`\`\`typescript\nimport { MCPClient } from "@uhg/ai-marketplace-sdk";\n\nconst client = new MCPClient({\n  agentId: "denial-agent",\n  apiKey: process.env.MARKETPLACE_API_KEY,\n  environment: "production",\n});\n\nconst response = await client.invoke({\n  input: { claimId: "CLM-2026-009812" },\n  context: { tenantId: "optum-prod" },\n});\n\nconsole.log(response.result);\n\`\`\`\n\nInstall the SDK with:\n\`\`\`bash\nnpm install @uhg/ai-marketplace-sdk\n\`\`\``,
};

function pickCanned(text: string, model: string): string {
  const t = text.toLowerCase();
  let raw =
    t.includes("deni") ? CANNED.deny :
    t.includes("code") || t.includes("snippet") || t.includes("sdk") ? CANNED.code :
    CANNED.default;
  return raw.replace("{model}", model);
}

export const usePlaygroundStore = create<PlaygroundStore>((set, get) => ({
  config: {
    model: "gpt-4o",
    agentId: null,
    agentName: "(No agent — direct model)",
    systemPrompt: DEFAULT_SYSTEM_PROMPT,
    temperature: 0.7,
    maxTokens: 2048,
    topP: 1,
    stream: true,
  },
  messages: [],
  isStreaming: false,
  activeTab: "preview",
  currentOutput: "",
  tokenCount: 0,
  logs: [],

  setConfig: (patch) =>
    set((s) => ({ config: { ...s.config, ...patch } })),

  addMessage: (msg) =>
    set((s) => ({ messages: [...s.messages, msg] })),

  setMessages: (messages) => set({ messages }),

  appendToOutput: (chunk) =>
    set((s) => ({ currentOutput: s.currentOutput + chunk })),

  finalizeOutput: (meta) => {
    const { currentOutput, messages, config } = get();
    const msg: ChatMessage = {
      id: `msg-${Date.now()}`,
      role: "assistant",
      content: currentOutput,
      createdAt: new Date().toISOString(),
      tokens: Math.ceil(currentOutput.length / 4),
      meta,
    };
    set((s) => ({
      messages: [...messages, msg],
      currentOutput: "",
      isStreaming: false,
      tokenCount: s.tokenCount + (msg.tokens ?? 0),
    }));
  },

  setStreaming: (isStreaming) => set({ isStreaming, currentOutput: isStreaming ? "" : get().currentOutput }),

  setActiveTab: (activeTab) => set({ activeTab }),

  addLog: (line) =>
    set((s) => ({ logs: [...s.logs.slice(-199), `[${new Date().toISOString()}] ${line}`] })),

  clearConversation: () =>
    set({ messages: [], logs: [], tokenCount: 0, currentOutput: "" }),
}));

/** Simulates a streaming response — replace with real SSE call when API is wired */
export async function sendMessage(userText: string): Promise<void> {
  const store = usePlaygroundStore.getState();
  const { config, addMessage, setStreaming, appendToOutput, finalizeOutput, addLog } = store;

  // 1. Push user message
  addMessage({
    id: `msg-${Date.now()}`,
    role: "user",
    content: userText,
    createdAt: new Date().toISOString(),
    tokens: Math.ceil(userText.length / 4),
  });

  addLog(`→ user message (${userText.length} chars)`);
  addLog(`→ routing to model=${config.model} agent=${config.agentId ?? "none"} temp=${config.temperature}`);

  setStreaming(true);
  const t0 = Date.now();

  const reply = pickCanned(userText, config.model);
  const words = reply.split(/(\s+)/);

  // Stream word-by-word
  for (const word of words) {
    await new Promise<void>((r) => setTimeout(r, 18 + Math.random() * 30));
    appendToOutput(word);
  }

  const latencyMs = Date.now() - t0;
  addLog(`← assistant reply (${reply.length} chars, ${latencyMs}ms)`);

  finalizeOutput({ model: config.model, latencyMs, finishReason: "stop" });
}
