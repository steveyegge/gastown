"use client";

import { createContext, useContext } from "react";

/**
 * Signals whether the MSAL PublicClientApplication has been
 * fully initialised and wrapped in MsalProvider.
 *
 * useMsal() / useIsAuthenticated() must only be called when this is true.
 */
export const MsalReadyContext = createContext<boolean>(false);

export function useMsalReady(): boolean {
  return useContext(MsalReadyContext);
}
