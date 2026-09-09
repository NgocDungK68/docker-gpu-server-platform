"use client";

import { CheckCircle2, CircleAlert, X } from "lucide-react";
import { createContext, useCallback, useContext, useMemo, useState } from "react";
import { cn } from "@/lib/utils/cn";

type ToastTone = "success" | "error" | "info";
interface ToastItem { id: number; title: string; description?: string; tone: ToastTone }
interface ToastContextValue { pushToast: (toast: Omit<ToastItem, "id">) => void }

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const pushToast = useCallback((toast: Omit<ToastItem, "id">) => {
    const id = Date.now() + Math.random();
    setToasts((current) => [...current, { ...toast, id }]);
    window.setTimeout(() => setToasts((current) => current.filter((item) => item.id !== id)), 4_500);
  }, []);
  const value = useMemo(() => ({ pushToast }), [pushToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed right-4 top-4 z-[100] flex w-[min(420px,calc(100vw-2rem))] flex-col gap-3" aria-live="polite">
        {toasts.map((toast) => (
          <div key={toast.id} className={cn("pointer-events-auto flex gap-3 rounded-xl border bg-white p-4 shadow-xl", toast.tone === "error" ? "border-red-200" : "border-slate-200")}>
            {toast.tone === "error" ? <CircleAlert className="mt-0.5 size-5 text-red-600" /> : <CheckCircle2 className={cn("mt-0.5 size-5", toast.tone === "success" ? "text-emerald-600" : "text-sky-600")} />}
            <div className="min-w-0 flex-1"><p className="font-semibold text-slate-950">{toast.title}</p>{toast.description && <p className="mt-1 text-sm text-slate-600">{toast.description}</p>}</div>
            <button className="self-start text-slate-400 hover:text-slate-800" onClick={() => setToasts((current) => current.filter((item) => item.id !== toast.id))} aria-label="Đóng thông báo"><X className="size-4" /></button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used inside ToastProvider");
  return context;
}
