"use client";

import { Bell, HelpCircle, User } from "lucide-react";

export function TopNav() {
  return (
    <header className="flex h-16 items-center justify-between border-b bg-white px-6">
      <div />
      <div className="flex items-center gap-3">
        <button className="rounded-lg p-2 text-gray-500 hover:bg-gray-100 transition-colors">
          <HelpCircle className="h-5 w-5" />
        </button>
        <button className="relative rounded-lg p-2 text-gray-500 hover:bg-gray-100 transition-colors">
          <Bell className="h-5 w-5" />
          <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-red-500" />
        </button>
        <button className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 transition-colors">
          <div className="flex h-7 w-7 items-center justify-center rounded-full bg-azure-500 text-white">
            <User className="h-4 w-4" />
          </div>
          <span>Sign in</span>
        </button>
      </div>
    </header>
  );
}
