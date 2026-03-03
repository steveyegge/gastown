import { NextRequest, NextResponse } from "next/server"
import { getModelCard } from "@/lib/foundry-client"

/**
 * GET /api/foundry/[modelId]
 *
 * Server-side proxy that calls Azure AI Foundry (or returns mock data)
 * for the given asset model ID. Credentials stay server-side.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ modelId: string }> }
) {
  const { modelId } = await params

  if (!modelId) {
    return NextResponse.json({ error: "modelId is required" }, { status: 400 })
  }

  try {
    const data = await getModelCard(modelId)
    return NextResponse.json(data, {
      headers: {
        // Cache for 5 minutes at the CDN / browser level
        "Cache-Control": "public, s-maxage=300, stale-while-revalidate=60",
      },
    })
  } catch (err) {
    console.error("[api/foundry] Unexpected error:", err)
    return NextResponse.json(
      { error: "Failed to fetch model card data" },
      { status: 500 }
    )
  }
}
