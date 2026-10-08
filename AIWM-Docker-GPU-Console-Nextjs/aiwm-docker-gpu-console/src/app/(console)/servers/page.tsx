"use client";

import Link from "next/link";
import { Search, Server as ServerIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { ServerMode } from "@/features/inventory/server-mode";
import { useServers } from "@/features/inventory/use-inventory";
import { relativeTime } from "@/lib/utils/format";

export default function ServersPage() {
  const query = useServers();
  const [search, setSearch] = useState("");
  const servers = useMemo(() => (query.data ?? []).filter((server) =>
    [server.name, server.machineId, server.address]
      .join(" ").toLowerCase().includes(search.toLowerCase())), [query.data, search]);

  return <><PageHeader title="Máy chủ GPU" />
    <Card><div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 p-4">
      <div className="relative w-full max-w-md"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" />
        <input aria-label="Tìm máy chủ" className="field-input pl-9" placeholder="Tên máy chủ, địa chỉ…" value={search} onChange={(event) => setSearch(event.target.value)} /></div>
      <p className="text-sm text-slate-500">{servers.length} máy chủ</p></div>
      {query.isLoading ? <TableSkeleton /> : query.error ? <div className="p-5"><ErrorState message="Không tải được danh sách máy chủ. Vui lòng thử lại." onRetry={query.refetch} /></div> :
        servers.length === 0 ? <EmptyState icon={<ServerIcon className="size-6" />} title="Không tìm thấy máy chủ" description="Đổi từ khóa hoặc kết nối thêm máy chủ." /> :
        <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Máy chủ / địa chỉ</th><th>Trạng thái</th><th>GPU sẵn sàng</th><th>Containers</th><th>Cập nhật gần nhất</th></tr></thead>
          <tbody>{servers.map((server) => {
            const free = server.schedulable ? server.gpus.filter((gpu) => gpu.healthy && gpu.state === "FREE").length : 0;
            const active = server.containers.filter((container) => ["running", "paused", "restarting"].includes(container.state));
            return <tr key={server.id}>
              <td><Link href={"/servers/" + server.id} className="font-bold text-slate-950 hover:underline">{server.name}</Link><p className="font-mono text-xs text-slate-500">{server.address || server.host?.hostname || server.machineId}</p></td>
              <td><Badge value={server.status} /><div className="mt-1"><ServerMode mode={server.capabilities?.operatingMode} /></div><p className="mt-1 max-w-56 text-xs text-slate-500">{server.schedulable ? "Có thể nhận workload" : "Chưa nhận workload mới"}</p></td>
              <td><b>{free}/{server.gpus.length}</b><p className="text-xs text-slate-500">{[...new Set(server.gpus.map((gpu) => gpu.model))].join(", ")}</p></td>
              <td>{server.capabilities?.dockerAvailable && server.status !== "OFFLINE" ? <><p>{active.filter((container) => container.origin === "MANAGED").length} Do AIWM quản lý</p><p>{active.filter((container) => container.origin !== "MANAGED").length} Có sẵn trên máy</p></> : <span className="text-slate-400">Chưa có dữ liệu</span>}</td>
              <td><p>{relativeTime(server.lastHeartbeatAt)}</p><p className="text-xs text-slate-500">{relativeTime(server.inventoryReceivedAt)}</p></td>
            </tr>;
          })}</tbody></table></div>}
    </Card></>;
}
