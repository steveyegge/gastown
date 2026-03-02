"use client";

// Thin wrapper — swap in real MSAL provider once Azure app registration is ready.
export function AuthProvider({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
