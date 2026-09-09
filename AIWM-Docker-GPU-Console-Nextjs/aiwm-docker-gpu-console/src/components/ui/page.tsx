import { cn } from "@/lib/utils/cn";

export function PageHeader({ eyebrow, title, description, actions }: { eyebrow?: string; title: string; description?: string; actions?: React.ReactNode }) {
  return <header className="page-header"><div className="min-w-0">{eyebrow && <p className="mb-1 text-xs font-bold uppercase tracking-[0.14em] text-[var(--brand-red-dark)]">{eyebrow}</p>}<h1 className="text-2xl font-bold tracking-tight text-slate-950 lg:text-[1.75rem]">{title}</h1>{description && <p className="mt-1.5 max-w-3xl text-sm leading-6 text-slate-600">{description}</p>}</div>{actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}</header>;
}

export function EmptyState({ icon, title, description, action, className }: { icon: React.ReactNode; title: string; description: string; action?: React.ReactNode; className?: string }) {
  return <div className={cn("flex min-h-64 flex-col items-center justify-center px-6 py-12 text-center", className)}><div className="mb-4 grid size-12 place-items-center rounded-2xl bg-slate-100 text-slate-500">{icon}</div><h3 className="font-semibold text-slate-950">{title}</h3><p className="mt-1 max-w-md text-sm leading-6 text-slate-500">{description}</p>{action && <div className="mt-5">{action}</div>}</div>;
}

export function ErrorState({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  return <div className="rounded-xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-800"><p className="font-semibold">Không tải được dữ liệu</p><p className="mt-1">{message ?? "Kiểm tra Control Plane và thử lại."}</p>{onRetry && <button className="mt-3 font-semibold underline" onClick={onRetry}>Tải lại</button>}</div>;
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return <div className="space-y-3 p-5">{Array.from({ length: rows }).map((_, index) => <div key={index} className="h-12 animate-pulse rounded-lg bg-slate-100" />)}</div>;
}
