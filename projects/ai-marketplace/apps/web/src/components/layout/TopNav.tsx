"use client";

import { Bell, HelpCircle, LogIn, LogOut, User } from "lucide-react";
import { useMsal, useIsAuthenticated } from "@azure/msal-react";
import { loginRequest } from "@/lib/auth/msal-config";
import { useMsalReady } from "@/lib/auth/msal-ready-context";

function AuthButtonInner() {
  const { instance, accounts } = useMsal();
  const isAuthenticated = useIsAuthenticated();
  const account = accounts[0];

  const handleSignIn = () =>
    instance.loginPopup(loginRequest).catch(console.error);

  const handleSignOut = () =>
    instance
      .logoutPopup({ account, postLogoutRedirectUri: "/" })
      .catch(console.error);

  const initials = account?.name
    ? account.name
        .split(" ")
        .map((n) => n[0])
        .slice(0, 2)
        .join("")
        .toUpperCase()
    : "";

  if (!isAuthenticated) {
    return (
      <button
        onClick={handleSignIn}
        className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-100 transition-colors"
        title="Sign in with Microsoft"
      >
        <div className="flex h-7 w-7 items-center justify-center rounded-full bg-azure-500 text-white">
          <LogIn className="h-4 w-4" />
        </div>
        <span>Sign in</span>
      </button>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <div className="flex flex-col items-end leading-none">
        <span className="text-sm font-medium text-gray-800 truncate max-w-[160px]">
          {account?.name ?? account?.username}
        </span>
        <span className="text-xs text-gray-400 truncate max-w-[160px]">
          {account?.username}
        </span>
      </div>
      <button
        className="flex h-8 w-8 items-center justify-center rounded-full bg-azure-500 text-white text-xs font-bold select-none"
        title={account?.name}
        tabIndex={-1}
      >
        {initials || <User className="h-4 w-4" />}
      </button>
      <button
        onClick={handleSignOut}
        className="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-red-500 transition-colors"
        title="Sign out"
      >
        <LogOut className="h-4 w-4" />
      </button>
    </div>
  );
}

/**
 * Sign-in / sign-out button — only mounts MSAL hooks once the
 * provider is ready to avoid "useMsal called outside MsalProvider" errors.
 */
function AuthButton() {
  const msalReady = useMsalReady();
  if (!msalReady) {
    return (
      <button className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-gray-400 cursor-default">
        <div className="flex h-7 w-7 items-center justify-center rounded-full bg-gray-200">
          <User className="h-4 w-4" />
        </div>
        <span>Sign in</span>
      </button>
    );
  }
  return <AuthButtonInner />;
}

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
        <AuthButton />
      </div>
    </header>
  );
}
