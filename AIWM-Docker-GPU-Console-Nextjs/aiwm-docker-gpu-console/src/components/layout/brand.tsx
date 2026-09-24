import Image from "next/image";

export function Brand({ compact = false }: { compact?: boolean }) {
  return <div className="flex min-w-0 items-center gap-3"><div className="grid h-9 w-[92px] place-items-center overflow-hidden"><Image src="/brand/viettel-logo.svg" alt="Viettel" width={92} height={20} priority /></div>{!compact && <><div className="h-7 w-px bg-slate-200" /><div className="min-w-0"><p className="truncate text-sm font-bold tracking-tight text-slate-950">AIWM</p><p className="truncate text-[11px] font-medium text-slate-500">Quản lý tài nguyên GPU</p></div></>}</div>;
}
