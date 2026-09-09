"use client";

import { X } from "lucide-react";
import { useEffect } from "react";
import { Button } from "@/components/ui/button";

export function Dialog({ open, title, description, confirmLabel = "Xác nhận", destructive = false, busy = false, onClose, onConfirm }: { open: boolean; title: string; description: string; confirmLabel?: string; destructive?: boolean; busy?: boolean; onClose: () => void; onConfirm: () => void }) {
  useEffect(() => {
    if (!open) return;
    const handler = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);
  if (!open) return null;
  return <div className="fixed inset-0 z-[90] grid place-items-center bg-slate-950/40 p-4 backdrop-blur-sm" role="presentation" onMouseDown={(event) => event.currentTarget === event.target && onClose()}><div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="dialog-title"><div className="flex items-start justify-between gap-4"><div><h2 id="dialog-title" className="text-lg font-bold text-slate-950">{title}</h2><p className="mt-2 text-sm leading-6 text-slate-600">{description}</p></div><button onClick={onClose} className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-900" aria-label="Đóng"><X className="size-5" /></button></div><div className="mt-6 flex justify-end gap-2"><Button variant="secondary" onClick={onClose} disabled={busy}>Huỷ</Button><Button variant={destructive ? "danger" : "primary"} onClick={onConfirm} disabled={busy}>{busy ? "Đang xử lý…" : confirmLabel}</Button></div></div></div>;
}
