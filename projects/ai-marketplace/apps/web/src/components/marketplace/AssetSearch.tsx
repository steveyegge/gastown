"use client";

interface Props {
  value: string;
  onChange: (value: string) => void;
}

export function AssetSearch({ value, onChange }: Props) {
  return (
    <div className="relative">
      <svg
        className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={2}
      >
        <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M17 11A6 6 0 1 1 5 11a6 6 0 0 1 12 0z" />
      </svg>
      <input
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Search agents, MCP servers, models…"
        className="w-full rounded-lg border border-gray-200 bg-white py-2.5 pl-9 pr-4 text-sm shadow-sm outline-none transition placeholder:text-gray-400 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
      />
    </div>
  );
}
