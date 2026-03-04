"use client"

import { type ReactNode, useEffect, useState } from "react"
import {
  PublicClientApplication,
  EventType,
  type AuthenticationResult,
} from "@azure/msal-browser"
import { MsalProvider } from "@azure/msal-react"
import { buildMsalConfig } from "@/lib/auth/msal-config"

export function AuthProvider({ children }: { children: ReactNode }) {
  const [instance, setInstance] = useState<PublicClientApplication | null>(null)

  useEffect(() => {
    // Fetch client ID + tenant ID from the server API route at runtime.
    // This avoids baking them in as NEXT_PUBLIC_ build-time vars, so the
    // same Docker image works across environments.
    fetch("/api/auth-config")
      .then((res) => res.json())
      .then(async ({ clientId, tenantId }: { clientId: string; tenantId: string }) => {
        const inst = new PublicClientApplication(buildMsalConfig(clientId, tenantId))
        await inst.initialize()

        // Restore active account from cache
        const accounts = inst.getAllAccounts()
        if (accounts.length > 0) inst.setActiveAccount(accounts[0])

        // Keep active account in sync after login
        inst.addEventCallback((event) => {
          if (event.eventType === EventType.LOGIN_SUCCESS && event.payload) {
            const payload = event.payload as AuthenticationResult
            inst.setActiveAccount(payload.account)
          }
        })

        setInstance(inst)
      })
      .catch(console.error)
  }, [])

  if (!instance) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-background">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-[var(--optum-orange)] border-t-transparent" />
      </div>
    )
  }

  return <MsalProvider instance={instance}>{children}</MsalProvider>
}
