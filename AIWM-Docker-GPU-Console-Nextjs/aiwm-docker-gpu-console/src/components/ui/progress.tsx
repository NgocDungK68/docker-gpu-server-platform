import { cn } from "@/lib/utils/cn";

export function Progress({ value, tone = "red", className }: { value: number; tone?: "red" | "blue" | "green" | "amber"; className?: string }) {
  const safeValue = Math.max(0, Math.min(value, 100));
  return <div className={cn("h-2 overflow-hidden rounded-full bg-slate-100", className)} role="progressbar" aria-valuenow={safeValue} aria-valuemin={0} aria-valuemax={100}><div className={cn("h-full rounded-full transition-all", tone === "red" && "bg-[var(--brand-red)]", tone === "blue" && "bg-sky-500", tone === "green" && "bg-emerald-500", tone === "amber" && "bg-amber-500")} style={{ width: `${safeValue}%` }} /></div>;
}
