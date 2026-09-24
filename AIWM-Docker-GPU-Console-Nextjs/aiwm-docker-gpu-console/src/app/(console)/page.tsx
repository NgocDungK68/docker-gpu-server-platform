"use client";

import Link from "next/link";
import { Activity, Cpu, RefreshCw, Server as ServerIcon, ShieldAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { MetricCard } from "@/components/dashboard/metric-card";
import { useDashboard } from "@/features/dashboard/use-dashboard";
import { fleetMetrics } from "@/features/dashboard/fleet";

const percent = (value: number | null) => value === null ? "—" : value.toLocaleString("vi-VN", { maximumFractionDigits: 1 }) + "%";

export default function DashboardPage() {
  const dashboard = useDashboard();
  const metrics = fleetMetrics(dashboard.servers);
  const rows = dashboard.admin
    ? dashboard.organizations.map(org => ({ id: org.id, name: org.code, detail: org.name, stats: fleetMetrics(dashboard.servers.filter(s => s.organizationId === org.id)), server: undefined }))
    : dashboard.servers.map(server => ({ id: server.id, name: server.name, detail: "", stats: fleetMetrics([server]), server }));

  return <>
    <PageHeader title="Tổng quan tài nguyên" actions={<Button variant="secondary" onClick={() => void dashboard.refetch()} disabled={dashboard.isLoading}><RefreshCw className="size-4" />Làm mới</Button>} />
    {dashboard.error ? <ErrorState message={dashboard.error.message} onRetry={dashboard.refetch} />
      : dashboard.isLoading ? <TableSkeleton rows={8} /> : <div className="space-y-5">
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard label="Máy chủ GPU" value={metrics.servers} note={metrics.online + " trực tuyến"} icon={ServerIcon} tone="blue" />
          <MetricCard label="GPU vật lý" value={metrics.total} note={metrics.reserved + " đã giữ tài nguyên"} icon={Cpu} />
          <MetricCard label="GPU sẵn sàng" value={metrics.free} icon={Cpu} tone="green" />
          <MetricCard label="GPU đang sử dụng" value={metrics.allocated} icon={Activity} tone="blue" />
          <MetricCard label="Đang dùng ngoài hệ thống" value={metrics.legacy} icon={Activity} tone="amber" />
          <MetricCard label="Chưa xác định" value={metrics.unknown} icon={ShieldAlert} tone="amber" />
          <MetricCard label="Không khả dụng" value={metrics.unhealthy} icon={ShieldAlert} />
          <MetricCard label="GPU Util trung bình" value={percent(metrics.averageUtilization)} note={metrics.telemetryCount ? metrics.telemetryCount + " GPU có số liệu" : "Chưa có số liệu"} icon={Activity} tone="green" />
        </div>
        <Card>
          <CardHeader title={dashboard.admin ? "Hiệu suất theo đơn vị" : "Tài nguyên theo máy chủ"} />
          <div className="overflow-x-auto"><table className="data-table">
            <thead><tr><th>{dashboard.admin ? "Đơn vị" : "Máy chủ"}</th>{dashboard.admin && <th>Máy chủ</th>}<th>GPU</th><th>Sẵn sàng</th><th>Đang sử dụng</th><th>Util TB</th>{!dashboard.admin && <th>Trạng thái</th>}</tr></thead>
            <tbody>{rows.map(row => <tr key={row.id}>
              <td><p className="font-bold text-slate-900">{row.server ? <Link href={"/servers/" + row.id}>{row.name}</Link> : row.name}</p>{row.detail && <p className="mt-1 text-xs text-slate-500">{row.detail}</p>}</td>
              {dashboard.admin && <td>{row.stats.servers}</td>}
              <td>{row.stats.total}</td><td className="font-semibold text-emerald-700">{row.stats.free}</td><td>{row.stats.allocated}</td><td>{percent(row.stats.averageUtilization)}</td>
              {row.server && <td><Badge value={row.server.status} /></td>}
            </tr>)}</tbody>
          </table></div>
          {!rows.length && <p className="p-6 text-sm text-slate-500">Chưa có tài nguyên trong phạm vi đang xem.</p>}
        </Card>
      </div>}
  </>;
}
