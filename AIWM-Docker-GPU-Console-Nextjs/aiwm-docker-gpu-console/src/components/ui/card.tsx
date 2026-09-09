import { cn } from "@/lib/utils/cn";
import type { ComponentPropsWithoutRef } from "react";

export function Card({ className, children, ...props }: ComponentPropsWithoutRef<"section">) {
  return <section {...props} className={cn("surface-card", className)}>{children}</section>;
}

export function CardHeader({ title, description, action, className }: { title: string; description?: string; action?: React.ReactNode; className?: string }) {
  return <div className={cn("flex items-start justify-between gap-4 border-b border-slate-100 px-5 py-4", className)}><div><h2 className="text-base font-semibold text-slate-950">{title}</h2>{description && <p className="mt-1 text-sm text-slate-500">{description}</p>}</div>{action}</div>;
}
