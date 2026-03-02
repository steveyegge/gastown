import type { Asset } from "@/lib/types";

interface Props {
  asset: Pick<Asset, "evaluations">;
}

export function AssetEvaluations({ asset }: Props) {
  if (!asset.evaluations.length) return null;

  return (
    <section className="rounded-xl border bg-white p-6">
      <h2 className="mb-4 font-semibold">Evaluation Results</h2>
      <div className="space-y-3">
        {asset.evaluations.map((ev) => (
          <div key={ev.id} className="flex items-center justify-between rounded-lg border p-3">
            <div className="min-w-0">
              <p className="font-medium text-sm text-gray-800 truncate">{ev.name}</p>
              <p className="text-xs text-gray-400">
                {ev.model} · {new Date(ev.runAt).toLocaleDateString()}
              </p>
            </div>
            <div className="flex items-center gap-3 shrink-0">
              <div className="w-20">
                <div className="h-1.5 w-full rounded-full bg-gray-100">
                  <div
                    className={`h-1.5 rounded-full ${ev.score >= 80 ? "bg-green-500" : ev.score >= 60 ? "bg-amber-400" : "bg-red-400"}`}
                    style={{ width: `${ev.score}%` }}
                  />
                </div>
              </div>
              <span className="w-9 text-right text-sm font-semibold">{ev.score}</span>
              <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${ev.passed ? "bg-green-50 text-green-700" : "bg-red-50 text-red-700"}`}>
                {ev.passed ? "Pass" : "Fail"}
              </span>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

