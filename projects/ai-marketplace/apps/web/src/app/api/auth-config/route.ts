/**
 * GET /api/auth-config
 *
 * Returns the public Azure AD / Entra ID auth configuration
 * to the browser at runtime — avoids baking secrets into the
 * Next.js build image.
 *
 * Reads server-side env vars (set in Container App environment or
 * .env.local for local dev).  AZURE_AD_CLIENT_ID and
 * AZURE_AD_TENANT_ID are populated by the IaC / azd deploy.
 */
export async function GET(request: Request) {
  const clientId = process.env.AZURE_AD_CLIENT_ID ?? "";
  const tenantId = process.env.AZURE_AD_TENANT_ID ?? "";

  if (!clientId || !tenantId) {
    return Response.json(
      { error: "Azure AD client ID or tenant ID not configured" },
      { status: 503 }
    );
  }

  return Response.json(
    { clientId, tenantId },
    {
      headers: {
        // Safe to cache briefly — values don't change between deploys
        "Cache-Control": "public, max-age=300, stale-while-revalidate=60",
      },
    }
  );
}
