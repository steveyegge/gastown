import { type Configuration, LogLevel } from "@azure/msal-browser"

/**
 * Builds MSAL configuration from values resolved at runtime.
 *
 * clientId and tenantId come from /api/auth-config (a Next.js API route that
 * reads AZURE_AD_CLIENT_ID / AZURE_AD_TENANT_ID server-side env vars).
 * These are public values — safe to expose in the browser.
 */
export function buildMsalConfig(clientId: string, tenantId: string): Configuration {
  return {
    auth: {
      clientId,
      authority: `https://login.microsoftonline.com/${tenantId}`,
      redirectUri: "/",
      postLogoutRedirectUri: "/",
    },
    cache: {
      cacheLocation: "sessionStorage",
      storeAuthStateInCookie: false,
    },
    system: {
      loggerOptions: {
        loggerCallback: (level: LogLevel, message: string, containsPii: boolean) => {
          if (containsPii) return
          if (process.env.NODE_ENV === "development") {
            switch (level) {
              case LogLevel.Error:   console.error("[MSAL]", message); break
              case LogLevel.Warning: console.warn("[MSAL]", message);  break
            }
          }
        },
      },
    },
  }
}

/** Scopes for the initial sign-in login request */
export const loginRequest = {
  scopes: ["openid", "profile", "email", "User.Read"],
}
