import type { Asset } from "@/lib/types";

const typeColors: Record<string, string> = {
  Agent: "text-blue-600 bg-blue-50",
  "MCP Server": "text-purple-600 bg-purple-50",
  Model: "text-green-600 bg-green-50",
  "Workflow Template": "text-orange-600 bg-orange-50",
  Evaluator: "text-yellow-600 bg-yellow-50",
  Connector: "text-gray-600 bg-gray-50",
};

interface Props {
  asset: Pick<Asset, "dependencies">;
}

export function AssetDependencies({ asset }: Props) {
  if (!asset.dependencies.length) return null;

  const required = asset.dependencies.filter((d) => d.required);
  const optional = asset.dependencies.filter((d) => !d.required);

  return (
    <section className="rounded-xl border bg-white p-6">
      <h2 className="mb-4 font-semibold">Dependencies</h2>

      {required.length > 0 && (
        <>
          <p className="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500">Required</p>
          <div className="mb-4 space-y-2">
            {required.map((dep) => (
              <DependencyRow key={dep.id} dep={dep} />
            ))}
          </div>
        </>
      )}

      {optional.length > 0 && (
        <>
          <p className="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500">Optional</p>
          <div className="space-y-2">
            {optional.map((dep) => (
              <DependencyRow key={dep.id} dep={dep} />
            ))}
          </div>
        </>
      )}
    </section>
  );
}

function DependencyRow({ dep }: { dep: Asset["dependencies"][number] }) {
  return (
    <div className="flex items-center justify-between rounded-lg border p-3">
      <div className="flex items-center gap-3">
        <span className={`rounded px-2 py-0.5 text-xs font-medium ${typeColors[dep.type] ?? "text-gray-600 bg-gray-50"}`}>
          {dep.type}
        </span>
        <span className="font-medium text-sm text-gray-800">{dep.name}</span>
      </div>
      <span className="font-mono text-xs text-gray-400">{dep.version}</span>
    </div>
  );
}
