import { NextResponse } from "next/server"

/**
 * Returns Azure AD auth config values for the browser-side MSAL client.
 * These are public values (not secrets) — everyone can see them in the
 * login redirect URL, so it is safe to expose them from a server route.
 */
export async function GET() {
  return NextResponse.json({
    clientId: process.env.AZURE_AD_CLIENT_ID ?? "",
    tenantId: process.env.AZURE_AD_TENANT_ID ?? "",
  })
}
