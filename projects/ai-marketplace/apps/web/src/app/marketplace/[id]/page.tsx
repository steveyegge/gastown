import { notFound } from "next/navigation";
import { fetchAsset } from "@/lib/api/assets";
import { AssetDetailHeader } from "@/components/marketplace/AssetDetailHeader";
import { AssetDependencies } from "@/components/marketplace/AssetDependencies";
import { AssetDeployButton } from "@/components/marketplace/AssetDeployButton";
import { AssetEvaluations } from "@/components/marketplace/AssetEvaluations";
import { RatingWidget } from "@/components/marketplace/RatingWidget";

interface Props {
  params: { id: string };
}

export default async function AssetDetailPage({ params }: Props) {
  let asset;
  try {
    asset = await fetchAsset(params.id);
  } catch {
    notFound();
  }

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <AssetDetailHeader asset={asset} />

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Main info */}
        <div className="lg:col-span-2 space-y-6">
          <section className="rounded-xl border bg-white p-6">
            <h2 className="mb-3 font-semibold">About</h2>
            <p className="text-sm text-gray-600 whitespace-pre-wrap">{asset.description}</p>
          </section>

          <AssetDependencies asset={asset} />
          <AssetEvaluations asset={asset} />

          {/* Changelog */}
          <section className="rounded-xl border bg-white p-6">
            <h2 className="mb-3 font-semibold">Changelog</h2>
            {asset.versions.map((v) => (
              <div key={v.version} className="mb-4 border-l-2 border-gray-200 pl-4">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm font-semibold">{v.version}</span>
                  {v.isLatest && (
                    <span className="rounded bg-green-100 px-1.5 py-0.5 text-xs text-green-700">
                      latest
                    </span>
                  )}
                  <span className="text-xs text-gray-400">{v.releasedAt}</span>
                </div>
                <p className="mt-1 text-sm text-gray-600">{v.notes}</p>
              </div>
            ))}
          </section>
        </div>

        {/* Sidebar */}
        <div className="space-y-4">
          <AssetDeployButton asset={asset} />
          <RatingWidget assetId={asset.id} />

          {/* Publisher */}
          <div className="rounded-xl border bg-white p-5">
            <h3 className="mb-3 text-sm font-semibold text-gray-700">Publisher</h3>
            <div className="flex items-center gap-3">
              <div className="h-10 w-10 rounded-full bg-gradient-to-br from-blue-400 to-blue-600 flex items-center justify-center text-white font-bold text-sm">
                {asset.publisher.name[0]}
              </div>
              <div>
                <p className="font-medium text-sm">{asset.publisher.name}</p>
                {asset.publisher.verified && (
                  <span className="text-xs text-green-600">✓ Verified publisher</span>
                )}
              </div>
            </div>
          </div>

          {/* Metadata */}
          <div className="rounded-xl border bg-white p-5 space-y-3 text-sm">
            <h3 className="font-semibold text-gray-700">Details</h3>
            <MetaRow label="Version" value={asset.latestVersion} />
            <MetaRow label="Type" value={asset.type} />
            <MetaRow label="License" value={asset.license} />
            <MetaRow label="Deployment" value={asset.deploymentModes.join(", ")} />
            <MetaRow label="Compliance" value={asset.complianceTier} />
          </div>
        </div>
      </div>
    </div>
  );
}

function MetaRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span className="text-gray-500">{label}</span>
      <span className="font-medium text-gray-900">{value}</span>
    </div>
  );
}
