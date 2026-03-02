import { NextRequest, NextResponse } from "next/server"
import {
  isAmlConfigured,
  listAmlModels,
  listAmlModelVersions,
  registerAmlModel,
  amlModelToMarketplace,
  type RegisterModelInput,
} from "@/lib/azure-ml-client"
import { models as staticModels } from "@/lib/models-data"

/**
 * GET /api/models
 *
 * Returns model list from:
 *  1. Static marketplace catalog (models-data.ts)
 *  2. Azure ML workspace if configured (AZURE_ML_WORKSPACE env set)
 *
 * Query params:
 *   source=all|static|azureml   (default: all)
 */
export async function GET(req: NextRequest) {
  const source = req.nextUrl.searchParams.get("source") ?? "all"

  let amlModels: ReturnType<typeof amlModelToMarketplace>[] = []

  if ((source === "all" || source === "azureml") && isAmlConfigured()) {
    try {
      const containers = await listAmlModels()
      const versionLists = await Promise.allSettled(
        containers.slice(0, 50).map((c) => listAmlModelVersions(c.name))
      )
      for (const result of versionLists) {
        if (result.status === "fulfilled") {
          // Take the latest version of each model
          const sorted = result.value.sort((a, b) =>
            (b.createdTime ?? "").localeCompare(a.createdTime ?? "")
          )
          if (sorted[0]) amlModels.push(amlModelToMarketplace(sorted[0]))
        }
      }
    } catch (err) {
      console.warn("[api/models] AML sync failed:", err)
    }
  }

  const combined =
    source === "azureml"
      ? amlModels
      : source === "static"
      ? staticModels
      : [...staticModels, ...amlModels]

  return NextResponse.json(
    {
      models: combined,
      meta: {
        total: combined.length,
        static: staticModels.length,
        azureml: amlModels.length,
        amlConfigured: isAmlConfigured(),
      },
    },
    {
      headers: { "Cache-Control": "public, s-maxage=120, stale-while-revalidate=30" },
    }
  )
}

/**
 * POST /api/models
 *
 * Registers a custom model in the Azure ML workspace.
 * Also adds it to the AI Asset Marketplace catalog as a "byom" (bring-your-own-model) entry.
 *
 * Body: RegisterModelInput JSON
 */
export async function POST(req: NextRequest) {
  if (!isAmlConfigured()) {
    return NextResponse.json(
      {
        error: "Azure ML workspace not configured",
        hint: "Set AZURE_SUBSCRIPTION_ID, AZURE_ML_RESOURCE_GROUP, and AZURE_ML_WORKSPACE environment variables.",
      },
      { status: 503 }
    )
  }

  let input: RegisterModelInput
  try {
    input = await req.json()
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 })
  }

  // Basic validation
  const required: (keyof RegisterModelInput)[] = ["name", "version", "description", "modelUri", "framework", "taskType"]
  for (const field of required) {
    if (!input[field]) {
      return NextResponse.json({ error: `Missing required field: ${field}` }, { status: 400 })
    }
  }

  try {
    const registered = await registerAmlModel(input)
    const marketplaceEntry = amlModelToMarketplace(registered)

    return NextResponse.json(
      {
        success: true,
        model: registered,
        marketplaceEntry,
      },
      { status: 201 }
    )
  } catch (err) {
    console.error("[api/models] Registration failed:", err)
    return NextResponse.json(
      { error: "Model registration failed", detail: String(err) },
      { status: 500 }
    )
  }
}
