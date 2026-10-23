"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/api-client";

function initialStart() {
  const date = new Date(Date.now() + 5 * 60000);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export function ContinuationDialog({ busy, error, onClose, onSubmit }: {
  busy: boolean; error: string | null; onClose: () => void;
  onSubmit: (input: { neededAt: string; ttlSeconds: number }) => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const [start, setStart] = useState(initialStart);
  const [hours, setHours] = useState("1");
  const [validation, setValidation] = useState<string | null>(null);
  const options = useQuery({ queryKey: ["job-options"], queryFn: () => api.getAllocationOptions() });
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => dialog?.close();
  }, []);
  const maxHours = options.data ? options.data.limits.maxTtlSeconds / 3600 : undefined;
  return <dialog ref={ref} aria-labelledby={titleId} onCancel={event => { event.preventDefault(); if (!busy) onClose(); }} className="m-auto w-[calc(100%_-_2rem)] max-w-md rounded-2xl bg-white p-6 text-slate-900 shadow-2xl backdrop:bg-slate-950/40">
    <h2 id={titleId} className="text-lg font-bold">Xin cấp phát tiếp</h2>
    <p className="mt-2 text-sm text-slate-500">Chọn thời gian bắt đầu và thời lượng mới.</p>
    <form className="mt-5 space-y-4" onSubmit={event => {
      event.preventDefault();
      const date = new Date(start);
      const duration = Math.round(Number(hours) * 3600);
      if (!start || !Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) { setValidation("Chọn thời điểm bắt đầu trong tương lai."); return; }
      if (!Number.isFinite(duration) || duration < 1 || !Number.isInteger(duration) || !options.data || duration > options.data.limits.maxTtlSeconds) { setValidation("Thời lượng sử dụng chưa hợp lệ."); return; }
      setValidation(null);
      onSubmit({ neededAt: date.toISOString(), ttlSeconds: duration });
    }}>
      <label className="block text-sm font-semibold">Bắt đầu mong muốn<input type="datetime-local" required className="field-input mt-2" value={start} disabled={busy} onChange={event => setStart(event.target.value)} /></label>
      <label className="block text-sm font-semibold">Thời lượng sử dụng (giờ)<input type="number" required min={0.01} step={0.01} max={maxHours} className="field-input mt-2" value={hours} disabled={busy} onChange={event => setHours(event.target.value)} /></label>
      {maxHours !== undefined && <p className="text-xs text-slate-500">Tối đa {maxHours.toLocaleString("vi-VN")} giờ</p>}
      {options.isError && <p role="alert" className="text-sm text-red-700">Chưa tải được lựa chọn thời lượng. <button type="button" className="underline" onClick={() => void options.refetch()}>Thử lại</button></p>}
      {(validation || error) && <p role="alert" className="text-sm text-red-700">{validation || error}</p>}
      <div className="flex justify-end gap-2 pt-2"><Button variant="secondary" disabled={busy} onClick={onClose}>Hủy</Button><Button type="submit" disabled={busy || !options.data}>{busy ? "Đang gửi…" : "Gửi yêu cầu"}</Button></div>
    </form>
  </dialog>;
}
