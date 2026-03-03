import { NextRequest, NextResponse } from "next/server"
import {
  isAmlConfigured,
  getAmlCompute,
  controlAmlCompute,
  deleteAmlCompute,
  type ComputeAction,
} from "@/lib/azure-ml-client"

interface RouteParams {
  params: Promise<{ name: string }>
}

/**
 * GET /api/aml/compute/[name]
 *
 * Returns details for a single compute resource.
 */
export async function GET(_req: NextRequest, { params }: RouteParams) {
  if (!isAmlConfigured()) {
    return NextResponse.json({ error: "Azure ML workspace not configured." }, { status: 503 })
  }
  const { name } = await params
  try {
    const compute = await getAmlCompute(name)
    return NextResponse.json(compute)
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return NextResponse.json({ error: message }, { status: 502 })
  }
}

/**
 * POST /api/aml/compute/[name]
 *
 * Perform a control action on a compute instance.
 *
 * Body: { action: "start" | "stop" | "restart" }
 */
export async function POST(req: NextRequest, { params }: RouteParams) {
  if (!isAmlConfigured()) {
    return NextResponse.json({ error: "Azure ML workspace not configured." }, { status: 503 })
  }
  const { name } = await params
  let body: { action: ComputeAction }
  try {
    body = (await req.json()) as { action: ComputeAction }
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 })
  }

  const validActions: ComputeAction[] = ["start", "stop", "restart"]
  if (!validActions.includes(body.action)) {
    return NextResponse.json(
      { error: `action must be one of: ${validActions.join(", ")}` },
      { status: 400 }
    )
  }

  try {
    const result = await controlAmlCompute(name, body.action)
    return NextResponse.json({ ...result, name, action: body.action }, { status: 202 })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    console.error(`[api/aml/compute/${name}] Action ${body.action} failed:`, message)
    return NextResponse.json({ error: message }, { status: 502 })
  }
}

/**
 * DELETE /api/aml/compute/[name]
 *
 * Delete a compute resource (also deletes underlying VM).
 */
export async function DELETE(_req: NextRequest, { params }: RouteParams) {
  if (!isAmlConfigured()) {
    return NextResponse.json({ error: "Azure ML workspace not configured." }, { status: 503 })
  }
  const { name } = await params
  try {
    await deleteAmlCompute(name)
    return NextResponse.json({ deleted: true, name }, { status: 202 })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    console.error(`[api/aml/compute/${name}] Delete failed:`, message)
    return NextResponse.json({ error: message }, { status: 502 })
  }
}
