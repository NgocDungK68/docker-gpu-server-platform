"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ChevronRight, Menu, Plus } from "lucide-react";
import { Brand } from "@/components/layout/brand";
import { ConnectionStatus } from "@/components/layout/connection-status";
import { flatNavigation } from "@/config/navigation";
import { AccountBadge } from "@/features/identity/session";

export function Topbar({ onMenu }: { onMenu: () => void }) {
  const pathname = usePathname();
  const current = [...flatNavigation].sort((a, b) => b.href.length - a.href.length).find((item) => item.href === "/" ? pathname === "/" : pathname.startsWith(item.href));
  return <header className="sticky top-0 z-30 flex h-[72px] items-center border-b border-slate-200 bg-white/95 px-4 backdrop-blur lg:px-7"><div className="flex items-center gap-3 lg:hidden"><button onClick={onMenu} className="rounded-lg p-2 text-slate-600 hover:bg-slate-100" aria-label="Mở menu"><Menu className="size-5" /></button><Brand compact /></div><div className="hidden min-w-0 items-center gap-2 text-sm lg:flex"><span className="font-medium text-slate-400">AIWM</span><ChevronRight className="size-4 text-slate-300" /><span className="truncate font-semibold text-slate-800">{current?.label ?? "Console"}</span></div><div className="ml-auto flex items-center gap-2.5"><div className="hidden md:block"><ConnectionStatus /></div><Link href="/workloads/new" className="btn btn-primary hidden sm:inline-flex"><Plus className="size-4" />Tạo workload</Link><AccountBadge /></div></header>;
}
