import {
  PublicClientApplication,
  type Configuration,
  BrowserCacheLocation,
} from "@azure/msal-browser";

export interface AuthConfigResponse {
  clientId: string;
  tenantId: string;
  redirectUri: string;
}

/**
 * Creates a fully-initialised MSAL PublicClientApplication from the
 * runtime auth config fetched from /api/auth-config.
 */
export function createMsalInstance(
  cfg: AuthConfigResponse
): PublicClientApplication {
  const msalConfig: Configuration = {
    auth: {
      clientId: cfg.clientId,
      authority: `https://login.microsoftonline.com/${cfg.tenantId}`,
      redirectUri: cfg.redirectUri,
    },
    cache: {
      cacheLocation: BrowserCacheLocation.LocalStorage,
      // Set true only if IE11 / cross-site-cookie issues arise
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
