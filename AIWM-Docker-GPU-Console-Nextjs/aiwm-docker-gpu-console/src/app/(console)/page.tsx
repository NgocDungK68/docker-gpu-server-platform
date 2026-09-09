"use client";

import Link from "next/link";
import { Activity, ArrowRight, Boxes, Cpu, RefreshCw, Server as ServerIcon, ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { MetricCard } from "@/components/dashboard/metric-card";
import { GPUStateChart } from "@/components/charts/gpu-state-chart";
import { ServerCapacityChart } from "@/components/charts/server-capacity-chart";
import { useDashboard } from "@/features/dashboard/use-dashboard";
import { formatPercent, relativeTime, shortId } from "@/lib/utils/format";

export default function DashboardPage() {
  const dashboard = useDashboard();
  const summary = dashboard.summary;
  const usablePct = summary?.gpusTotal ? (summary.gpusFree / summary.gpusTotal) * 100 : 0;
  const protectedCount = (summary?.gpusOccupiedLegacy ?? 0) + (summary?.gpusOccupiedUnknown ?? 0);
  const recentJobs = [...dashboard.jobs].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 5);

  return <><PageHeader eyebrow="Logical Resource Pool" title="Tổng quan vận hành" description="Một góc nhìn tập trung cho các Docker GPU server, workload cũ và job do AIWM điều phối." actions={<Button variant="secondary" onClick={() => dashboard.refetch()} disabled={dashboard.isLoading}><RefreshCw className="size-4" />Làm mới</Button>} />
    {dashboard.error ? <ErrorState message={dashboard.error.message} onRetry={dashboard.refetch} /> : <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="Máy chủ online" value={`${summary?.serversOnline ?? 0}/${summary?.serversTotal ?? 0}`} note="Agent đang heartbeat đúng hạn" icon={ServerIcon} tone="green" />
        <MetricCard label="GPU sẵn sàng" value={`${summary?.gpusFree ?? 0}/${summary?.gpusTotal ?? 0}`} note={`${formatPercent(usablePct)} tổng pool có thể cấp phát`} icon={Cpu} tone="red" />
        <MetricCard label="GPU được bảo vệ" value={protectedCount} note="Legacy và consumer chưa xác định" icon={ShieldAlert} tone="amber" />
        <MetricCard label="Jobs" value={`${summary?.jobsRunning ?? 0} / ${summary?.jobsQueued ?? 0}`} note="Đang chạy / đang chờ" icon={Boxes} tone="blue" />
      </div>

      <div className="grid gap-5 xl:grid-cols-[1fr_1.15fr]">
        <Card><CardHeader title="Trạng thái GPU" description="Ảnh chụp inventory mới nhất của toàn pool" action={<Link href="/gpus" className="text-sm font-bold text-[var(--brand-red-dark)]">Xem chi tiết</Link>} />{dashboard.isLoading ? <TableSkeleton rows={4} /> : <GPUStateChart items={dashboard.gpus} />}</Card>
        <Card><CardHeader title="Phân bổ theo máy chủ" description="AIWM, workload được bảo vệ và GPU còn trống" action={<Link href="/servers" className="text-sm font-bold text-[var(--brand-red-dark)]">Quản lý máy chủ</Link>} />{dashboard.isLoading ? <TableSkeleton rows={4} /> : <ServerCapacityChart servers={dashboard.servers} />}</Card>
      </div>

      <div className="grid gap-5 xl:grid-cols-[1.35fr_.65fr]">
        <Card><CardHeader title="Workload gần đây" description="Trạng thái actual được Agent đồng bộ về Control Plane" action={<Link href="/workloads" className="inline-flex items-center gap-1 text-sm font-bold text-[var(--brand-red-dark)]">Tất cả <ArrowRight className="size-4" /></Link>} />{dashboard.isLoading ? <TableSkeleton /> : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Workload</th><th>Trạng thái</th><th>GPU</th><th>Chiến lược</th><th>Cập nhật</th></tr></thead><tbody>{recentJobs.map((job) => <tr key={job.id}><td><Link href={`/workloads/${job.id}`} className="font-semibold text-slate-950 hover:text-[var(--brand-red-dark)]">{job.name}</Link><p className="mt-0.5 font-mono text-xs text-slate-400">{shortId(job.id)}</p></td><td><Badge value={job.status} /></td><td>{job.resources.gpuCount} × {job.resources.gpuModel || "Bất kỳ"}</td><td className="capitalize">{job.strategy}</td><td>{relativeTime(job.updatedAt)}</td></tr>)}</tbody></table></div>}</Card>
        <Card><CardHeader title="Cần chú ý" description="Suy ra từ trạng thái hiện tại" /><div className="space-y-3 p-4">
          {dashboard.servers.filter((server) => !server.schedulable).map((server) => <Link href={`/servers/${server.id}`} key={server.id} className="flex gap-3 rounded-xl border border-amber-200 bg-amber-50 p-3.5 transition hover:border-amber-300"><Activity className="mt-0.5 size-5 shrink-0 text-amber-600" /><div><p className="text-sm font-bold text-slate-900">{server.name} đang {server.status.toLowerCase()}</p><p className="mt-1 text-xs leading-5 text-slate-600">{server.schedulingReason} · heartbeat {relativeTime(server.lastHeartbeatAt)}.</p></div></Link>)}
          {(summary?.gpusOccupiedUnknown ?? 0) > 0 && <Link href="/gpus?state=OCCUPIED_UNKNOWN" className="flex gap-3 rounded-xl border border-red-200 bg-red-50 p-3.5 transition hover:border-red-300"><ShieldAlert className="mt-0.5 size-5 shrink-0 text-red-600" /><div><p className="text-sm font-bold text-slate-900">Có GPU chưa xác định consumer</p><p className="mt-1 text-xs leading-5 text-slate-600">AIWM bảo vệ GPU này và không đưa vào pool cho tới khi làm rõ.</p></div></Link>}
          {!dashboard.isLoading && dashboard.servers.every((server) => server.schedulable) && (summary?.gpusOccupiedUnknown ?? 0) === 0 && <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-800">Không có cảnh báo inventory quan trọng.</div>}
        </div></Card>
      </div>
      <Card id="recent-events"><CardHeader title="Sự kiện job gần đây" description="Chuyển trạng thái và lý do từ Control Plane" /><div className="divide-y divide-slate-100">{(summary?.recentEvents ?? []).slice(0, 10).map((event, index) => <Link key={event.jobId + event.at + index} href={"/workloads/" + event.jobId} className="flex flex-wrap items-center gap-3 p-4 text-sm hover:bg-slate-50"><Badge value={event.status} /><span className="font-mono text-xs">{shortId(event.jobId)}</span><span className="flex-1 text-slate-600">{event.reason}</span><time className="text-xs text-slate-400">{relativeTime(event.at)}</time></Link>)}{!summary?.recentEvents?.length && <p className="p-5 text-sm text-slate-500">Chưa có sự kiện job.</p>}</div></Card>
    </div>}
  </>;
}
