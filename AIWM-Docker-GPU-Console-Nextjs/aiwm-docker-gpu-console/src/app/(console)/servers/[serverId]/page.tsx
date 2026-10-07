"use client";

import Link from "next/link";
import { Activity, ArrowLeft, Box, Cpu, RefreshCw } from "lucide-react";
import { useParams } from "next/navigation";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { MetricCard } from "@/components/dashboard/metric-card";
import { useToast } from "@/components/ui/toast";
import { useDrainServer, useServer } from "@/features/inventory/use-inventory";
import { ServerMode } from "@/features/inventory/server-mode";
import { useSession } from "@/features/identity/session";
import { identityApi } from "@/lib/api/identity";
import { formatDateTime, formatMiB, formatPercent } from "@/lib/utils/format";

function Availability({ available }: { available?: boolean }) {
  return <Badge value={available === undefined ? "OFFLINE" : available ? "FREE" : "RESERVED"}
    label={available === undefined ? "Chưa xác định" : available ? "Sẵn sàng" : "Không khả dụng"} />;
}

export default function ServerDetailPage() {
  const { serverId } = useParams<{ serverId: string }>();
  const query = useServer(serverId);
  const mutation = useDrainServer(serverId);
  const session = useSession();
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: session.user.role === "ADMIN" });
  const { pushToast } = useToast();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const server = query.data;

  if (query.isLoading) return <><PageHeader title="Đang tải máy chủ…" /><Card><TableSkeleton rows={7} /></Card></>;
  if (query.error || !server) return <><PageHeader title="Không tìm thấy máy chủ" /><ErrorState message={query.error?.message} onRetry={query.refetch} /></>;

  const caps = server.capabilities;
  const organization = session.user.role === "ADMIN"
    ? organizations.data?.find(org => org.id === server.organizationId)
    : session.organization;
  const free = server.schedulable ? server.gpus.filter(gpu => gpu.healthy && gpu.state === "FREE").length : 0;
  const currentGPUData = caps?.gpuInventoryAvailable && server.status !== "OFFLINE";
  const telemetry = currentGPUData ? server.gpus.filter(gpu => gpu.healthy && Number.isFinite(gpu.utilizationPct) && gpu.utilizationPct >= 0 && gpu.utilizationPct <= 100) : [];
  const utilization = telemetry.length ? formatPercent(telemetry.reduce((total, gpu) => total + gpu.utilizationPct, 0) / telemetry.length) : "—";
  const containerDataAvailable = caps?.dockerAvailable && server.status !== "OFFLINE";
  const active = server.containers.filter(container => ["running", "paused", "restarting"].includes(container.state));
  const checks = [
    ["Kết nối Docker", caps?.dockerAvailable],
    ["Quan sát GPU", caps?.gpuInventoryAvailable],
    ["Driver NVIDIA", caps?.nvmlAvailable],
    ["Chạy container GPU", caps?.nvidiaContainerSupport],
    ["Nhận workload mới", caps ? server.schedulable : undefined],
  ] as const;
  const platformInfo = [
    ["Hệ điều hành / kiến trúc", [caps?.os || server.host?.os, caps?.architecture || server.host?.architecture].filter(Boolean).join(" / ") || "—"],
    ["Kernel", caps?.kernelVersion || "—"],
    ["Cgroup", caps?.cgroupMode || "—"],
    ["CPU / RAM", (server.host?.cpuCount ?? "—") + " CPU / " + (server.host?.memoryTotalMiB ? formatMiB(server.host.memoryTotalMiB) : "—")],
    ["Docker / API", [caps?.dockerVersion, caps?.dockerApiVersion].filter(Boolean).join(" / ") || "—"],
    ["Driver NVIDIA", caps?.nvidiaDriverVersion || "—"],
    ["CUDA Driver", caps?.cudaDriverVersion || "—"],
    ["Agent", server.agentVersion || "—"],
    ["Cập nhật gần nhất", formatDateTime(server.lastHeartbeatAt)],
  ];
  const commitDrain = () => mutation.mutate(!server.drained, {
    onSuccess: (updated) => { setConfirmOpen(false); pushToast({ tone: "success", title: updated.drained ? "Đã ngừng nhận workload mới" : "Đã mở nhận workload" }); },
    onError: (error) => pushToast({ tone: "error", title: "Không cập nhật được", description: error.message }),
  });

  return <>
    <Link href="/servers" className="mb-4 inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-950"><ArrowLeft className="size-4" />Danh sách máy chủ</Link>
    <PageHeader title={server.name} description={organization?.name} actions={<>
      {organization && <Badge value="MANAGED" label={organization.code} />}
      <Badge value={server.status} />
      <ServerMode mode={caps?.operatingMode} />
      <Button variant="secondary" onClick={() => void query.refetch()} disabled={query.isFetching}><RefreshCw className="size-4" />Làm mới</Button>
      <Button variant={server.drained ? "secondary" : "primary"} onClick={() => setConfirmOpen(true)}>{server.drained ? "Nhận workload mới" : "Ngừng nhận workload"}</Button>
    </>} />
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="GPU vật lý" value={server.gpus.length} note={!currentGPUData ? "Lần ghi nhận gần nhất" : undefined} icon={Cpu} tone="blue" />
        <MetricCard label="GPU sẵn sàng" value={free} icon={Cpu} tone="green" />
        <MetricCard label="Container đang chạy" value={containerDataAvailable ? active.length : "—"} icon={Box} />
        <MetricCard label="GPU Util trung bình" value={utilization} icon={Activity} tone="blue" />
      </div>
      <Card><CardHeader title="Khả năng hoạt động" />
        <dl className="grid gap-x-8 px-5 pb-2 sm:grid-cols-2 xl:grid-cols-3">{checks.map(([label, available]) =>
          <div key={label} className="flex items-center justify-between gap-3 border-t border-slate-100 py-4 text-sm"><dt className="font-medium text-slate-600">{label}</dt><dd><Availability available={available} /></dd></div>)}</dl>
      </Card>
      <Card><details className="p-5"><summary className="cursor-pointer font-bold text-slate-900">Thông tin nền tảng</summary>
        <dl className="mt-3 grid gap-x-8 sm:grid-cols-2 xl:grid-cols-3">{platformInfo.map(([label, value]) =>
          <div key={label} className="border-t border-slate-100 py-3 text-sm"><dt className="text-slate-500">{label}</dt><dd className="mt-1 break-words font-semibold text-slate-900">{value}</dd></div>)}</dl>
      </details></Card>
      <Card><CardHeader title="GPU trên máy chủ" description={!currentGPUData && server.gpus.length ? "Hiển thị lần ghi nhận gần nhất" : undefined} />
        {!server.gpus.length ? <p className="p-6 text-sm text-slate-500">Chưa có dữ liệu GPU.</p> : <div className="overflow-x-auto"><table className="data-table"><thead><tr>
          <th>GPU</th><th>VRAM đã dùng / Tổng</th><th>Mức sử dụng</th><th>Nhiệt độ</th><th>Trạng thái</th>
        </tr></thead><tbody>{server.gpus.map(gpu => <tr key={gpu.uuid}>
          <td><p className="font-semibold text-slate-950">{gpu.model}</p><p className="text-xs text-slate-500">GPU {gpu.index}</p></td>
          <td>{formatMiB(gpu.memoryUsedMiB)} / {formatMiB(gpu.memoryTotalMiB)}</td>
          <td>{formatPercent(gpu.utilizationPct)}</td><td>{gpu.temperatureC == null ? "—" : gpu.temperatureC + " °C"}</td>
          <td><Badge value={gpu.state} /></td>
        </tr>)}</tbody></table></div>}
      </Card>
      <Card><CardHeader title="Containers" />
        {!containerDataAvailable ? <p className="p-6 text-sm text-slate-500">Không có dữ liệu container.</p>
          : !server.containers.length ? <p className="p-6 text-sm text-slate-500">Chưa có container.</p>
            : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Container</th><th>Loại</th><th>Image</th><th>GPU</th><th>Trạng thái</th></tr></thead>
              <tbody>{server.containers.map(container => <tr key={container.id}>
                <td className="font-semibold text-slate-950">{container.name}</td><td><Badge value={container.origin} /></td>
                <td className="max-w-64 truncate" title={container.image}>{container.image}</td><td>{container.gpuUuids?.length ?? 0}</td><td><Badge value={container.state} /></td>
              </tr>)}</tbody></table></div>}
      </Card>
    </div>
    <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} onConfirm={commitDrain} busy={mutation.isPending}
      title={server.drained ? "Mở nhận workload mới?" : "Ngừng nhận workload mới?"}
      description={server.drained ? "Máy chủ sẽ tiếp tục nhận workload mới khi sẵn sàng." : "Máy chủ ngừng nhận workload mới. Workload đang chạy vẫn tiếp tục."}
      confirmLabel={server.drained ? "Nhận workload mới" : "Xác nhận"} />
  </>;
}
