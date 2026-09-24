import type { LucideIcon } from "lucide-react";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils/cn";

export function MetricCard({ label, value, note, icon: Icon, tone = "red" }: { label: string; value: string | number; note?: string; icon: LucideIcon; tone?: "red" | "green" | "blue" | "amber" }) {
  return <Card className="relative p-5"><div className={cn("absolute inset-x-0 top-0 h-0.5", tone === "red" && "bg-[var(--brand-red)]", tone === "green" && "bg-emerald-500", tone === "blue" && "bg-sky-500", tone === "amber" && "bg-amber-500")} /><div className="flex items-start justify-between gap-4"><div><p className="text-sm font-semibold text-slate-500">{label}</p><p className="mt-2 text-3xl font-bold tracking-tight text-slate-950">{value}</p>{note && <p className="mt-2 text-xs leading-5 text-slate-500">{note}</p>}</div><div className={cn("grid size-10 place-items-center rounded-xl", tone === "red" && "bg-red-50 text-[var(--brand-red)]", tone === "green" && "bg-emerald-50 text-emerald-600", tone === "blue" && "bg-sky-50 text-sky-600", tone === "amber" && "bg-amber-50 text-amber-600")}><Icon className="size-5" /></div></div></Card>;
}
