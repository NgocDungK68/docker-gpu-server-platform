import type { Metadata } from "next";
import "./globals.css";
import { AppProviders } from "@/providers/app-providers";

export const metadata: Metadata = {
  title: { default: "AIWM GPU Console", template: "%s · AIWM" },
  description: "Viettel AI Workload Manager for standalone Docker GPU servers",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="vi">
      <body><AppProviders>{children}</AppProviders></body>
    </html>
  );
}
