import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  typescript: { ignoreBuildErrors: true },
  images: {
    unoptimized: true,
    domains: ["avatars.githubusercontent.com", "storage.azure.com"],
  },
  env: {
    NEXT_PUBLIC_API_BASE_URL: process.env.API_BASE_URL ?? "http://localhost:7071/api",
    NEXT_PUBLIC_AZURE_CLIENT_ID: process.env.AZURE_CLIENT_ID ?? "",
    NEXT_PUBLIC_AZURE_TENANT_ID: process.env.AZURE_TENANT_ID ?? "",
  },
};

export default nextConfig;