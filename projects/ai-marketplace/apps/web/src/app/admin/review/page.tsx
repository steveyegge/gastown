"use client";

import { useState, useEffect } from "react";
import Link from "next/link";

interface Submission {
  id: string;
  assetName: string;
  type: string;
  version: string;
  status: string;
  submittedAt: string;
}

export default function AdminReviewPage() {
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Submission | null>(null);
  const [action, setAction] = useState<"approve" | "reject" | null>(null);
  const [notes, setNotes] = useState("");
  const [distributionScope, setDistributionScope] = useState<"internal" | "external">("internal");
  const [submitting, setSubmitting] = useState(false);

  const apiBase = process.env.NEXT_PUBLIC_API_URL ?? "/api";

  useEffect(() => {
    fetch(`${apiBase}/submissions?status=submitted`)
      .then((r) => r.json())
      .then((d: { items: Submission[] }) => setSubmissions(d.items ?? []))
      .finally(() => setLoading(false));
  }, [apiBase]);

  const handleAction = async () => {
    if (!selected || !action) return;
    setSubmitting(true);
    const endpoint = `${apiBase}/submissions/${selected.id}/${action}`;
    await fetch(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(
        action === "approve"
          ? { notes, distributionScope }
          : { reason: notes }
      ),
    });
    setSubmissions((s) => s.filter((sub) => sub.id !== selected.id));
    setSelected(null);
    setAction(null);
    setNotes("");
    setSubmitting(false);
  };

  return (
    <div className="mx-auto max-w-5xl space-y-6 py-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Submission Review Queue</h1>
          <p className="text-sm text-gray-500">Review and approve or reject publisher asset submissions.</p>
        </div>
        <span className="rounded-full bg-blue-50 px-3 py-1 text-sm font-semibold text-blue-700">
          {submissions.length} pending
        </span>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-16 animate-pulse rounded-xl bg-gray-100" />)}
        </div>
      ) : submissions.length === 0 ? (
        <div className="rounded-xl border bg-white p-12 text-center text-gray-400">
          No submissions pending review
        </div>
      ) : (
        <div className="space-y-2">
          {submissions.map((sub) => (
            <div
              key={sub.id}
              className={`flex items-center justify-between rounded-xl border bg-white p-4 transition hover:border-blue-200 cursor-pointer ${selected?.id === sub.id ? "border-blue-300 ring-2 ring-blue-100" : ""}`}
              onClick={() => { setSelected(sub); setAction(null); setNotes(""); }}
            >
              <div className="flex items-center gap-4">
                <span className="rounded bg-blue-50 px-2.5 py-1 text-xs font-semibold text-blue-700">{sub.type}</span>
                <div>
                  <p className="font-medium text-gray-900">{sub.assetName}</p>
                  <p className="text-xs text-gray-400">v{sub.version} · submitted {new Date(sub.submittedAt).toLocaleDateString()}</p>
                </div>
              </div>
              <span className="rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700">
                {sub.status}
              </span>
            </div>
          ))}
        </div>
      )}

      {/* Review panel */}
      {selected && (
        <div className="rounded-xl border bg-white p-6 space-y-4">
          <h2 className="font-semibold">Review: {selected.assetName}</h2>

          {!action ? (
            <div className="flex gap-3">
              <button
                onClick={() => setAction("approve")}
                className="rounded-lg bg-green-600 px-4 py-2 text-sm font-semibold text-white hover:bg-green-700"
              >
                Approve
              </button>
              <button
                onClick={() => setAction("reject")}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
              >
                Reject
              </button>
              <button onClick={() => setSelected(null)} className="rounded-lg border px-4 py-2 text-sm text-gray-600">
                Cancel
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {action === "approve" && (
                <div className="space-y-1">
                  <label className="block text-sm font-medium text-gray-700">Distribution scope</label>
                  <select
                    value={distributionScope}
                    onChange={(e) => setDistributionScope(e.target.value as "internal" | "external")}
                    className="rounded-lg border border-gray-200 px-3 py-2 text-sm"
                  >
                    <option value="internal">Internal only</option>
                    <option value="external">External (customer distribution)</option>
                  </select>
                </div>
              )}
              <div className="space-y-1">
                <label className="block text-sm font-medium text-gray-700">
                  {action === "approve" ? "Approval notes (optional)" : "Rejection reason *"}
                </label>
                <textarea
                  rows={3}
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                  className="w-full rounded-lg border border-gray-200 px-3 py-2 text-sm"
                  placeholder={action === "approve" ? "Any notes for the publisher…" : "Explain why this submission was rejected…"}
                />
              </div>
              <div className="flex gap-3">
                <button
                  onClick={handleAction}
                  disabled={submitting || (action === "reject" && !notes.trim())}
                  className={`rounded-lg px-4 py-2 text-sm font-semibold text-white disabled:opacity-60 ${action === "approve" ? "bg-green-600 hover:bg-green-700" : "bg-red-600 hover:bg-red-700"}`}
                >
                  {submitting ? "Submitting…" : `Confirm ${action}`}
                </button>
                <button onClick={() => setAction(null)} className="rounded-lg border px-4 py-2 text-sm text-gray-600">
                  Back
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
