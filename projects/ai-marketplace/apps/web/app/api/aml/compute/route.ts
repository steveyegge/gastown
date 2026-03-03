import { NextRequest, NextResponse } from "next/server"
import {
  isAmlConfigured,
  listAmlCompute,
  provisionComputeInstance,
  COMPUTE_VM_SIZES,
  type ProvisionComputeInput,
} from "@/lib/azure-ml-client"

/**
 * GET /api/aml/compute
 *
 * Returns all compute instances and clusters from the configured Azure ML workspace
 * (ai-project-q2w5uxlkh4c6o).
 *
 * Response:
 *   { computes: AmlComputeInstance[], configured: boolean, workspace: string }
 */
export async function GET() {
  if (!isAmlConfigured()) {
    return NextResponse.json({
      computes: [],
      configured: false,
      workspace: null,
      vmSizes: COMPUTE_VM_SIZES,
      error: "Azure ML workspace not configured. Set AZURE_SUBSCRIPTION_ID, AZURE_ML_RESOURCE_GROUP, AZURE_ML_WORKSPACE.",
    })
  }

  try {
    const computes = await listAmlCompute()
    return NextResponse.json(
      {
        computes,
        configured: true,
        workspace: process.env.AZURE_ML_WORKSPACE,
        resourceGroup: process.env.AZURE_ML_RESOURCE_GROUP,
        subscription: process.env.AZURE_SUBSCRIPTION_ID,
        vmSizes: COMPUTE_VM_SIZES,
      },
      {
        headers: { "Cache-Control": "no-store" },
      }
    )
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    console.error("[api/aml/compute] Failed to list computes:", message)
    return NextResponse.json(
      {
        computes: [],
        configured: true,
        workspace: process.env.AZURE_ML_WORKSPACE,
        vmSizes: COMPUTE_VM_SIZES,
        error: message,
      },
      { status: 502 }
    )
  }
}

/**
 * POST /api/aml/compute
 *
 * Provision a new ComputeInstance in the Azure ML workspace.
 *
 * Body: ProvisionComputeInput
 *   { name: string, vmSize: string, description?: string, enableSsh?: boolean, tags?: Record<string,string> }
 */
export async function POST(req: NextRequest) {
  if (!isAmlConfigured()) {
    return NextResponse.json(
      { error: "Azure ML workspace not configured." },
      { status: 503 }
    )
  }

  let input: ProvisionComputeInput
  try {
    input = (await req.json()) as ProvisionComputeInput
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 })
  }

  if (!input.name || !input.vmSize) {
    return NextResponse.json(
      { error: "name and vmSize are required" },
      { status: 400 }
    )
  }

  // Sanitize name — AML names must be alphanumeric + hyphens, 2-32 chars
  if (!/^[a-zA-Z][a-zA-Z0-9-]{1,31}$/.test(input.name)) {
    return NextResponse.json(
      { error: "name must start with a letter, contain only letters/numbers/hyphens, and be 2-32 characters" },
      { status: 400 }
    )
  }

  try {
    const result = await provisionComputeInstance(input)
    return NextResponse.json(result, { status: 202 })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    console.error("[api/aml/compute] Provision failed:", message)
    return NextResponse.json({ error: message }, { status: 502 })
  }
}
