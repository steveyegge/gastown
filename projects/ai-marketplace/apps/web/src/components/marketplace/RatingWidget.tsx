"use client";

import { useState } from "react";

interface Props {
  assetId: string;
  currentScore?: number; // user's existing score, if any
}

export function RatingWidget({ assetId, currentScore }: Props) {
  const [hovered, setHovered] = useState(0);
  const [submitted, setSubmitted] = useState<number | null>(currentScore ?? null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleRate(score: number) {
    if (loading) return;
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/assets/${assetId}/ratings`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ score, review: "" }),
      });
      if (!res.ok) throw new Error("Failed to submit rating");
      setSubmitted(score);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setLoading(false);
    }
  }

  const display = hovered || submitted || 0;

  return (
    <div className="rounded-xl border bg-white p-5 space-y-3">
      <h3 className="text-sm font-semibold text-gray-700">Rate This Asset</h3>

      {submitted ? (
        <p className="text-sm text-green-600">
          You rated this {submitted} star{submitted !== 1 ? "s" : ""}. Thank you!
        </p>
      ) : (
        <p className="text-xs text-gray-500">Click a star to submit your rating.</p>
      )}

      <div
        className="flex gap-1"
        onMouseLeave={() => setHovered(0)}
        aria-label="Star rating"
      >
        {[1, 2, 3, 4, 5].map((star) => (
          <button
            key={star}
            disabled={loading}
            onClick={() => handleRate(star)}
            onMouseEnter={() => setHovered(star)}
            aria-label={`Rate ${star} star${star !== 1 ? "s" : ""}`}
            className={[
              "text-2xl transition-transform hover:scale-110 focus:outline-none disabled:cursor-wait",
              display >= star ? "text-amber-400" : "text-gray-300",
            ].join(" ")}
          >
            ★
          </button>
        ))}
      </div>

      {error && <p className="text-xs text-red-600">{error}</p>}
    </div>
  );
}
