"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Upload, CheckCircle, Clock, XCircle, AlertCircle } from "lucide-react";

interface Submission {
  id: string;
  assetName: string;
  assetType: string;
  status: "draft" | "submitted" | "approved" | "rejected";
  submittedAt: string;
  rejectionReason?: string;
}

const STATUS_CONFIG = {
  draft:     { label: "Draft",     icon: Clock,         color: "text-gray-500",  bg: "bg-gray-100" },
  submitted: { label: "In Review", icon: AlertCircle,   color: "text-blue-600",  bg: "bg-blue-50" },
  approved:  { label: "Approved",  icon: CheckCircle,   color: "text-green-600", bg: "bg-green-50" },
  rejected:  { label: "Rejected",  icon: XCircle,       color: "text-red-600",   bg: "bg-red-50" },
};

export default function PublisherDashboardPage() {
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function load() {
      try {
        // Fetch all statuses for the current publisher
        const res = await fetch("/api/submissions?status=all");
        if (!res.ok) throw new Error("Failed to load submissions");
        const data = await res.json();
        setSubmissions(data.items ?? []);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Unknown error");
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  return (
    <div className="mx-auto max-w-4xl space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Publisher Dashboard</h1>
          <p className="mt-1 text-sm text-gray-500">
            Manage your published assets and track review status.
          </p>
        </div>
        <Link
          href="/publisher/register"
          className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-blue-700"
        >
          <Upload className="h-4 w-4" />
          Publish New Asset
        </Link>
      </div>

      {/* Stats strip */}
      <div className="grid grid-cols-4 gap-4">
        {(["draft", "submitted", "approved", "rejected"] as const).map((status) => {
          const cfg = STATUS_CONFIG[status];
          const Icon = cfg.icon;
          const count = submissions.filter((s) => s.status === status).length;
          return (
            <div key={status} className={`rounded-xl border p-4 ${cfg.bg}`}>
              <div className={`flex items-center gap-2 ${cfg.color}`}>
                <Icon className="h-4 w-4" />
                <span className="text-xs font-medium">{cfg.label}</span>
              </div>
              <p className={`mt-2 text-2xl font-bold ${cfg.color}`}>{count}</p>
            </div>
          );
        })}
      </div>

      {/* Submission list */}
      <section className="rounded-xl border bg-white">
        <div className="border-b px-6 py-4">
          <h2 className="font-semibold text-gray-900">Your Submissions</h2>
        </div>

        {loading && (
          <div className="flex items-center justify-center p-12 text-sm text-gray-400">
            Loading…
          </div>
        )}

        {error && (
          <div className="p-6 text-sm text-red-600">{error}</div>
        )}

        {!loading && !error && submissions.length === 0 && (
          <div className="flex flex-col items-center justify-center gap-3 p-12 text-center">
            <Upload className="h-10 w-10 text-gray-300" />
            <p className="text-sm text-gray-500">
              No submissions yet.{" "}
              <Link href="/publisher/register" className="text-blue-600 hover:underline">
                Publish your first asset
              </Link>
              .
            </p>
          </div>
        )}

        {!loading && submissions.length > 0 && (
          <ul className="divide-y">
            {submissions.map((sub) => {
              const cfg = STATUS_CONFIG[sub.status];
              const Icon = cfg.icon;
              return (
                <li key={sub.id} className="flex items-center gap-4 px-6 py-4">
                  <div className="min-w-0 flex-1">
                    <p className="font-medium text-gray-900 truncate">{sub.assetName}</p>
                    <p className="text-xs text-gray-400 mt-0.5">
                      {sub.assetType} · Submitted {new Date(sub.submittedAt).toLocaleDateString()}
                    </p>
                    {sub.rejectionReason && (
                      <p className="mt-1 text-xs text-red-600">
                        Reason: {sub.rejectionReason}
                      </p>
                    )}
                  </div>
                  <span
                    className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${cfg.bg} ${cfg.color}`}
                  >
                    <Icon className="h-3 w-3" />
                    {cfg.label}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
