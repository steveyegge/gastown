import {
  PublicClientApplication,
  type Configuration,
  BrowserCacheLocation,
} from "@azure/msal-browser";

export interface AuthConfigResponse {
  clientId: string;
  tenantId: string;
}

/**
 * Creates a fully-initialised MSAL PublicClientApplication from the
 * runtime auth config fetched from /api/auth-config.
 *
 * redirectUri is intentionally omitted — MSAL defaults to
 * window.location.href which is always the correctly registered URL.
 */
export function createMsalInstance(
  cfg: AuthConfigResponse
): PublicClientApplication {
  const msalConfig: Configuration = {
    auth: {
      clientId: cfg.clientId,
      authority: `https://login.microsoftonline.com/${cfg.tenantId}`,
      // No redirectUri set — MSAL uses window.location.href (the SPA URL),
      // which is exactly what's registered in the Entra ID app registration.
    },
    cache: {
      cacheLocation: BrowserCacheLocation.LocalStorage,
      storeAuthStateInCookie: false,
    },
    system: {
      allowNativeBroker: false,
    },
  };
  return new PublicClientApplication(msalConfig);
}

/** Default scopes for sign-in — profile + basic Graph read */
export const loginRequest = {
  scopes: ["openid", "profile", "User.Read"],
};
