"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutGrid,
  PlayCircle,
  Upload,
  ShieldCheck,
  BarChart2,
  Settings,
  Bot,
  Zap,
} from "lucide-react";
import { clsx } from "clsx";

const NAV_ITEMS = [
  { href: "/", label: "Home", icon: LayoutGrid },
  { href: "/marketplace", label: "Marketplace", icon: Bot },
  { href: "/orchestrator", label: "Orchestrator", icon: PlayCircle },
  { href: "/playground", label: "Playground", icon: Zap },
  { href: "/publisher", label: "Publish", icon: Upload },
  { href: "/admin/review", label: "Review Queue", icon: ShieldCheck },
  { href: "/governance", label: "Governance", icon: ShieldCheck },
  { href: "/analytics", label: "Analytics", icon: BarChart2 },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="flex h-full w-56 flex-shrink-0 flex-col border-r bg-white">
      {/* Logo */}
      <div className="flex h-16 items-center gap-2.5 border-b px-5">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-azure-500 text-white">
          <Bot className="h-5 w-5" />
        </div>
        <div>
          <p className="text-sm font-bold text-gray-900">AI Asset Marketplace</p>
          <p className="text-xs text-gray-400">v0.9 · MVP</p>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto p-3 space-y-0.5">
        {NAV_ITEMS.map(({ href, label, icon: Icon }) => {
          const isActive = pathname === href || (href !== "/" && pathname.startsWith(href));
          return (
            <Link
              key={href}
              href={href}
              className={clsx(
                "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                isActive
                  ? "bg-azure-50 text-azure-700"
                  : "text-gray-600 hover:bg-gray-100 hover:text-gray-900"
              )}
            >
              <Icon className="h-4 w-4 flex-shrink-0" />
              {label}
            </Link>
          );
        })}
      </nav>

      {/* Footer */}
      <div className="border-t p-3">
        <Link
          href="/settings"
          className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-gray-600 hover:bg-gray-100 hover:text-gray-900 transition-colors"
        >
          <Settings className="h-4 w-4" />
          Settings
        </Link>
      </div>
    </aside>
  );
}
