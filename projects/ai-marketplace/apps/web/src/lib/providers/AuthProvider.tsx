"use client";

import { useEffect, useState } from "react";
import { MsalProvider } from "@azure/msal-react";
import { type PublicClientApplication } from "@azure/msal-browser";
import {
  createMsalInstance,
  type AuthConfigResponse,
} from "@/lib/auth/msal-config";
import { MsalReadyContext } from "@/lib/auth/msal-ready-context";

/**
 * Fetches Azure AD config at runtime from /api/auth-config
 * (so clientId / tenantId are never baked into the image),
 * creates the MSAL PublicClientApplication, then wraps the
 * component tree with MsalProvider.
 *
 * Renders children immediately so the app shell never blocks on auth,
 * then re-renders with MsalProvider once MSAL is ready.
 */
export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [msalInstance, setMsalInstance] =
    useState<PublicClientApplication | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function initMsal() {
      try {
        const res = await fetch("/api/auth-config");
        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new Error(body?.error ?? `auth-config returned ${res.status}`);
        }
        const cfg: AuthConfigResponse = await res.json();
        const instance = createMsalInstance(cfg);
        await instance.initialize();
        // Process redirect response if returning from loginRedirect flow
        await instance.handleRedirectPromise();
        if (!cancelled) setMsalInstance(instance);
      } catch (err) {
        // Log and silently degrade — app still loads without auth
        console.error("[AuthProvider] MSAL init failed:", err);
      }
    }

    initMsal();
    return () => {
      cancelled = true;
    };
  }, []);

  if (!msalInstance) {
    // Render children without MSAL while config is loading
    return <>{children}</>;
  }

  return (
    <MsalReadyContext.Provider value={true}>
      <MsalProvider instance={msalInstance}>{children}</MsalProvider>
    </MsalReadyContext.Provider>
  );
}
