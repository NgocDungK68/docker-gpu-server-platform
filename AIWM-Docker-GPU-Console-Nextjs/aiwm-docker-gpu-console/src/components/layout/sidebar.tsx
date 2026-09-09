"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { X } from "lucide-react";
import { Brand } from "@/components/layout/brand";
import { ConnectionStatus } from "@/components/layout/connection-status";
import { navigation } from "@/config/navigation";
import { cn } from "@/lib/utils/cn";

export function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const pathname = usePathname();
  return <><div className={cn("fixed inset-0 z-40 bg-slate-950/40 backdrop-blur-sm lg:hidden", open ? "block" : "hidden")} onClick={onClose} aria-hidden="true" /><aside className={cn("fixed inset-y-0 left-0 z-50 flex w-[268px] flex-col border-r border-slate-200 bg-white transition-transform lg:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}><div className="flex h-[72px] items-center justify-between border-b border-slate-100 px-5"><Brand /><button className="rounded-lg p-2 text-slate-500 hover:bg-slate-100 lg:hidden" onClick={onClose} aria-label="Đóng menu"><X className="size-5" /></button></div><nav className="flex-1 overflow-y-auto px-3 py-5" aria-label="Điều hướng chính">{navigation.map((group) => <div key={group.label} className="mb-6"><p className="mb-2 px-3 text-[11px] font-bold uppercase tracking-[0.14em] text-slate-400">{group.label}</p><div className="space-y-1">{group.items.map((item) => { const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href) && !(item.href === "/workloads" && pathname === "/workloads/new"); const Icon = item.icon; return <Link key={item.href} href={item.href} onClick={onClose} className={cn("group flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-semibold transition", active ? "bg-red-50 text-[var(--brand-red-dark)]" : "text-slate-600 hover:bg-slate-50 hover:text-slate-950")}><Icon className={cn("size-[18px]", active ? "text-[var(--brand-red)]" : "text-slate-400 group-hover:text-slate-700")} /><span className="flex-1">{item.label}</span>{"planned" in item && item.planned && <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-slate-500">Sau</span>}</Link>; })}</div></div>)}</nav><div className="border-t border-slate-100 p-4"><ConnectionStatus /><p className="mt-3 text-xs leading-5 text-slate-400">AIWM Console v0.1<br />Docker GPU Resource Pool</p></div></aside></>;
}
