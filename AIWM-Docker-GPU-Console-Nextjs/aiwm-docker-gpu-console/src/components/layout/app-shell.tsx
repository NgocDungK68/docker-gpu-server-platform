"use client";

import { useState } from "react";
import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";
import { SessionGate } from "@/features/identity/session";
import { OrganizationContext } from "@/features/identity/organization-context";

export function AppShell({ children }: { children: React.ReactNode }) {
  return <SessionGate><AuthenticatedShell>{children}</AuthenticatedShell></SessionGate>;
}

function AuthenticatedShell({ children }: { children: React.ReactNode }) {
  const [menuOpen, setMenuOpen] = useState(false);
  return <div className="min-h-screen bg-[#f5f6f8]"><Sidebar open={menuOpen} onClose={() => setMenuOpen(false)} /><div className="lg:pl-[268px]"><Topbar onMenu={() => setMenuOpen(true)} /><main className="mx-auto w-full max-w-[1600px] p-4 sm:p-6 lg:p-7"><OrganizationContext />{children}</main></div></div>;
}
