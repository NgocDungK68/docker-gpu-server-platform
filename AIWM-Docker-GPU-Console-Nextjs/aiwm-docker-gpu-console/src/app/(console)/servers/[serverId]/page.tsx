"use client";

import Link from "next/link";
import { ArrowLeft, Box, Cpu } from "lucide-react";
import { useParams } from "next/navigation";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { Progress } from "@/components/ui/progress";
import { useToast } from "@/components/ui/toast";
import { useDrainServer, useServer } from "@/features/inventory/use-inventory";
import { formatDateTime, formatMiB, formatPercent, shortId } from "@/lib/utils/format";

export default function ServerDetailPage() {
  const { serverId } = useParams<{ serverId: string }>();
  const query = useServer(serverId);
  const mutation = useDrainServer(serverId);
  const { pushToast } = useToast();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const server = query.data;

  if (query.isLoading) return <><PageHeader title="Đang tải máy chủ…" /><Card><TableSkeleton rows={7} /></Card></>;
  if (query.error || !server) return <><PageHeader title="Không tìm thấy máy chủ" /><ErrorState message={query.error?.message} /></>;

  const used = server.gpus.filter((gpu) => gpu.state !== "FREE").length;
  const commitDrain = () => mutation.mutate(!server.drained, { onSuccess: (updated) => { setConfirmOpen(false); pushToast({ tone: "success", title: updated.drained ? "Đã ngừng nhận workload mới" : "Đã mở nhận workload", description: "Container đang chạy không bị thay đổi." }); }, onError: (error) => pushToast({ tone: "error", title: "Không cập nhật được", description: error.message }) });

  return <><Link href="/servers" className="mb-4 inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-950"><ArrowLeft className="size-4" />Danh sách máy chủ</Link><PageHeader  title={server.name} actions={<><Badge value={server.status} /><Button variant={server.drained ? "secondary" : "primary"} onClick={() => setConfirmOpen(true)}>{server.drained ? "Nhận workload mới" : "Ngừng nhận workload"}</Button></>} />
    <div className="grid gap-5 xl:grid-cols-[1fr_2fr]">
      <div className="space-y-5"><Card><CardHeader title="Thông tin máy chủ" /><dl className="divide-y divide-slate-100 px-5">{[["Nhận workload", server.schedulable ? "Sẵn sàng" : "Tạm ngừng"], ["Host", server.host?.hostname || "—"], ["OS / kiến trúc", [server.host?.os, server.host?.architecture].filter(Boolean).join(" / ") || "—"], ["CPU / RAM", (server.host?.cpuCount ?? 0) + " CPU / " + formatMiB(server.host?.memoryTotalMiB ?? 0)], ["Cập nhật tài nguyên", formatDateTime(server.inventoryReceivedAt)], ["Docker", server.dockerVersion ?? "—"], ["Địa chỉ", server.address ?? "—"], ["Agent version", server.agentVersion ?? "—"], ["Kết nối gần nhất", formatDateTime(server.lastHeartbeatAt)]].map(([term, value]) => <div key={term} className="flex justify-between gap-4 py-3.5 text-sm"><dt className="text-slate-500">{term}</dt><dd className="text-right font-semibold text-slate-900">{value}</dd></div>)}</dl></Card><Card><CardHeader title="Nhãn máy chủ" /><div className="flex flex-wrap gap-2 p-5">{Object.entries(server.labels ?? {}).map(([key, value]) => <span key={key} className="rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 font-mono text-xs text-slate-700">{key}={value}</span>)}</div></Card></div>
      <div className="space-y-5"><Card><CardHeader title="GPU trên máy chủ" description={`${used}/${server.gpus.length} GPU không sẵn sàng`} /><div className="grid gap-3 p-4 md:grid-cols-2">{server.gpus.map((gpu) => { const memoryPct = gpu.memoryTotalMiB ? (gpu.memoryUsedMiB / gpu.memoryTotalMiB) * 100 : 0; return <div key={gpu.uuid} className="rounded-xl border border-slate-200 p-4"><div className="flex items-start justify-between gap-3"><div className="flex gap-3"><div className="grid size-9 place-items-center rounded-lg bg-slate-100 text-slate-600"><Cpu className="size-[18px]" /></div><div><p className="font-bold text-slate-950">GPU {gpu.index} · {gpu.model}</p><p className="mt-0.5 font-mono text-xs text-slate-400">{shortId(gpu.uuid, 18)}</p></div></div><Badge value={gpu.state} /></div><div className="mt-4"><div className="mb-1.5 flex justify-between text-xs text-slate-500"><span>VRAM {formatMiB(gpu.memoryUsedMiB)} / {formatMiB(gpu.memoryTotalMiB)}</span><b>{formatPercent(memoryPct)}</b></div><Progress value={memoryPct} tone={memoryPct > 85 ? "red" : "blue"} /></div><div className="mt-3 flex justify-between text-xs text-slate-500"><span>GPU util <b className="text-slate-900">{formatPercent(gpu.utilizationPct)}</b></span><span>Nhiệt độ <b className="text-slate-900">{gpu.temperatureC ?? "—"}°C</b></span></div></div>; })}</div></Card>
      <Card><CardHeader title="Containers" />{server.containers.length === 0 ? <div className="p-8 text-center text-sm text-slate-500">Chưa có container.</div> : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Container</th><th>Nguồn</th><th>Image</th><th>GPU</th><th>Trạng thái</th></tr></thead><tbody>{server.containers.map((container) => <tr key={container.id}><td><div className="flex items-center gap-2"><Box className="size-4 text-slate-400" /><div><p className="font-semibold text-slate-950">{container.name}</p><p className="font-mono text-xs text-slate-400">{shortId(container.id)}</p></div></div></td><td><Badge value={container.origin} /></td><td className="max-w-64 truncate font-mono text-xs">{container.image}</td><td>{container.gpuUuids?.length ?? 0}</td><td><Badge value={container.state} /></td></tr>)}</tbody></table></div>}</Card></div>
    </div><Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} onConfirm={commitDrain} busy={mutation.isPending} title={server.drained ? "Mở nhận workload mới?" : "Ngừng nhận workload này?"} description={server.drained ? "Máy chủ sẽ tiếp tục nhận workload mới." : "Máy chủ ngừng nhận workload mới. Workload đang chạy vẫn tiếp tục."} confirmLabel={server.drained ? "Nhận workload mới" : "Xác nhận"} /></>;
}
