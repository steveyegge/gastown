import type { Metadata } from "next";
import "./globals.css";
import { TopNav } from "@/components/layout/TopNav";
import { Sidebar } from "@/components/layout/Sidebar";
import { Toaster } from "sonner";
import { QueryProvider } from "@/lib/providers/QueryProvider";
import { AuthProvider } from "@/lib/providers/AuthProvider";

export const metadata: Metadata = {
  title: "AI Asset Marketplace",
  description:
    "Enterprise-grade marketplace for AI Agents, MCP Servers, Models, and Workflow Templates",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="bg-gray-50 text-gray-900 antialiased">
        <AuthProvider>
          <QueryProvider>
            <div className="flex h-screen overflow-hidden">
              <Sidebar />
              <div className="flex flex-1 flex-col overflow-hidden">
                <TopNav />
                <main className="flex-1 overflow-y-auto p-6">{children}</main>
              </div>
            </div>
            <Toaster position="top-right" richColors />
          </QueryProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
