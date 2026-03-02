"use client";

import { useState } from "react";

interface FormState {
  name: string;
  orgName: string;
  contactEmail: string;
  website: string;
  description: string;
  dataHandlingDeclaration: string;
}

const empty: FormState = {
  name: "", orgName: "", contactEmail: "",
  website: "", description: "", dataHandlingDeclaration: "",
};

export default function PublisherRegisterPage() {
  const [form, setForm] = useState<FormState>(empty);
  const [status, setStatus] = useState<"idle" | "submitting" | "done" | "error">("idle");
  const [result, setResult] = useState<{ id: string; publisherKey: string } | null>(null);
  const [error, setError] = useState("");

  const set = (key: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((f) => ({ ...f, [key]: e.target.value }));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setStatus("submitting");
    setError("");
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL ?? "/api"}/publishers`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(form),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error ?? "Registration failed");
      }
      const data = await res.json();
      setResult(data);
      setStatus("done");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      setStatus("error");
    }
  };

  if (status === "done" && result) {
    return (
      <div className="mx-auto max-w-lg py-16 text-center space-y-4">
        <div className="text-5xl">🎉</div>
        <h1 className="text-2xl font-bold">Registration submitted</h1>
        <p className="text-gray-500">Your publisher application is pending verification. Keep your publisher key safe.</p>
        <div className="rounded-lg bg-gray-50 border p-4 text-left space-y-2">
          <p className="text-xs text-gray-500 uppercase font-semibold tracking-wide">Publisher ID</p>
          <p className="font-mono text-sm">{result.id}</p>
          <p className="text-xs text-gray-500 uppercase font-semibold tracking-wide mt-3">Publisher Key (save this!)</p>
          <p className="font-mono text-sm break-all">{result.publisherKey}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6 py-8">
      <div>
        <h1 className="text-2xl font-bold">Become a publisher</h1>
        <p className="mt-1 text-gray-500">Register your organization to publish agents, tools, and models to the marketplace.</p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4 rounded-xl border bg-white p-6">
        <Field label="Display name" required>
          <input required value={form.name} onChange={set("name")} className={inputCls} placeholder="Acme AI" />
        </Field>
        <Field label="Organization name" required>
          <input required value={form.orgName} onChange={set("orgName")} className={inputCls} placeholder="Acme Corp" />
        </Field>
        <Field label="Contact email" required>
          <input required type="email" value={form.contactEmail} onChange={set("contactEmail")} className={inputCls} placeholder="ai@acme.com" />
        </Field>
        <Field label="Website">
          <input type="url" value={form.website} onChange={set("website")} className={inputCls} placeholder="https://acme.com" />
        </Field>
        <Field label="Publisher description" required>
          <textarea required rows={3} value={form.description} onChange={set("description")} className={inputCls} placeholder="Describe your organization and the AI assets you publish…" />
        </Field>
        <Field label="Data handling declaration" required hint="Declare how you handle PII, telemetry, and data egress in your assets.">
          <textarea required rows={3} value={form.dataHandlingDeclaration} onChange={set("dataHandlingDeclaration")} className={inputCls} placeholder="Our assets do not transmit PII externally. Telemetry is opt-in…" />
        </Field>

        {status === "error" && (
          <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-sm text-red-700">{error}</div>
        )}

        <button
          type="submit"
          disabled={status === "submitting"}
          className="w-full rounded-lg bg-blue-600 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-60"
        >
          {status === "submitting" ? "Submitting…" : "Submit publisher registration"}
        </button>
      </form>
    </div>
  );
}

const inputCls = "w-full rounded-lg border border-gray-200 px-3 py-2 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100";

function Field({ label, required, hint, children }: { label: string; required?: boolean; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-gray-700">
        {label} {required && <span className="text-red-500">*</span>}
      </label>
      {hint && <p className="text-xs text-gray-400">{hint}</p>}
      {children}
    </div>
  );
}
