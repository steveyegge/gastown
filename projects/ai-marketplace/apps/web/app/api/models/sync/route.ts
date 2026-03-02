import { NextResponse } from "next/server"
import {
  isAmlConfigured,
  listAmlModels,
  listAmlModelVersions,
  amlModelToMarketplace,
} from "@/lib/azure-ml-client"

/**
 * GET /api/models/sync
 *
 * Pulls the full model catalog from the Azure ML workspace and returns it
 * as a list of AI Asset Marketplace ModelData objects.
 *
 * Returns 503 when AML is not configured (env vars absent).
 */
export async function GET() {
  if (!isAmlConfigured()) {
    return NextResponse.json(
      {
        configured: false,
        message:
          "Azure ML workspace not configured. Set AZURE_SUBSCRIPTION_ID, AZURE_ML_RESOURCE_GROUP, and AZURE_ML_WORKSPACE.",
        models: [],
      },
      { status: 503 }
    )
  }

  try {
    const containers = await listAmlModels()

    const versionLists = await Promise.allSettled(
      containers.map((c) => listAmlModelVersions(c.name))
    )

    const syncedModels = []
    const errors: string[] = []

    for (let i = 0; i < containers.length; i++) {
      const result = versionLists[i]
      if (result.status === "fulfilled") {
        // Return ALL versions for each model
        for (const v of result.value) {
          syncedModels.push(amlModelToMarketplace(v))
        }
      } else {
        errors.push(`${containers[i].name}: ${result.reason}`)
      }
    }

    return NextResponse.json(
      {
        configured: true,
        workspace: process.env.AZURE_ML_WORKSPACE,
        resourceGroup: process.env.AZURE_ML_RESOURCE_GROUP,
        subscriptionId: process.env.AZURE_SUBSCRIPTION_ID,
        syncedAt: new Date().toISOString(),
        totalModels: containers.length,
        totalVersions: syncedModels.length,
        errors,
        models: syncedModels,
      },
      {
        headers: { "Cache-Control": "no-store" },
      }
    )
  } catch (err) {
    console.error("[api/models/sync] Error:", err)
    return NextResponse.json(
      { error: "Sync failed", detail: String(err) },
      { status: 500 }
    )
  }
}
