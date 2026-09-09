"use client";

import { useState } from "react";
import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";

export function AppShell({ children }: { children: React.ReactNode }) {
  const [menuOpen, setMenuOpen] = useState(false);
  return <div className="min-h-screen bg-[#f5f6f8]"><Sidebar open={menuOpen} onClose={() => setMenuOpen(false)} /><div className="lg:pl-[268px]"><Topbar onMenu={() => setMenuOpen(true)} /><main className="mx-auto w-full max-w-[1600px] p-4 sm:p-6 lg:p-7">{children}</main></div></div>;
}
