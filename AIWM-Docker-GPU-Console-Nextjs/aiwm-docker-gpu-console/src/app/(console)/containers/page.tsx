"use client";

import Link from "next/link";
import { Box, Container as ContainerIcon, Info, Search, ShieldCheck } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useContainers } from "@/features/inventory/use-inventory";
import type { ContainerOrigin } from "@/lib/api/types";
import { formatDateTime, shortId } from "@/lib/utils/format";
import { cn } from "@/lib/utils/cn";

const tabs: Array<{ value: "ALL" | ContainerOrigin; label: string }> = [{ value: "ALL", label: "Tất cả" }, { value: "MANAGED", label: "AIWM managed" }, { value: "LEGACY", label: "Workload cũ" }, { value: "UNKNOWN", label: "Chưa xác định" }];

export default function ContainersPage() {
  const [origin, setOrigin] = useState<"ALL" | ContainerOrigin>("ALL");
  const [search, setSearch] = useState("");
  const query = useContainers(origin === "ALL" ? undefined : origin);
  const items = useMemo(() => (query.data ?? []).filter((item) => `${item.container.name} ${item.container.image} ${item.container.id} ${item.serverName}`.toLowerCase().includes(search.toLowerCase())), [query.data, search]);

  return <><PageHeader eyebrow="Observed state" title="Docker containers" description="Inventory thực tế từ Agent. AIWM chỉ điều khiển container do chính hệ thống tạo và coi workload cũ là read-only." />
    <div className="mb-4 flex gap-3 rounded-xl border border-violet-200 bg-violet-50 p-4 text-sm text-violet-900"><ShieldCheck className="mt-0.5 size-5 shrink-0 text-violet-600" /><p><b>Ranh giới an toàn:</b> container không có nhãn <code className="rounded bg-white/70 px-1.5 py-0.5">aiwm.managed=true</code> không bao giờ nhận lệnh stop từ Control Plane.</p></div>
    <Card><div className="border-b border-slate-100 px-4 pt-4"><div className="flex gap-1 overflow-x-auto">{tabs.map((tab) => <button key={tab.value} onClick={() => setOrigin(tab.value)} className={cn("border-b-2 px-3 pb-3 pt-1 text-sm font-bold whitespace-nowrap", origin === tab.value ? "border-[var(--brand-red)] text-[var(--brand-red-dark)]" : "border-transparent text-slate-500 hover:text-slate-900")}>{tab.label}</button>)}</div></div><div className="border-b border-slate-100 p-4"><div className="relative max-w-md"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" /><input className="field-input pl-9" placeholder="Tìm container, image hoặc server…" value={search} onChange={(event) => setSearch(event.target.value)} /></div></div>
      {query.isLoading ? <TableSkeleton rows={6} /> : query.error ? <div className="p-5"><ErrorState message={query.error.message} onRetry={query.refetch} /></div> : items.length === 0 ? <EmptyState icon={<ContainerIcon className="size-6" />} title="Chưa có container" description="Agent chưa quan sát thấy container phù hợp với bộ lọc." /> : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Container</th><th>Nguồn</th><th>Trạng thái</th><th>Image</th><th>Máy chủ</th><th>GPU UUID</th><th>Bắt đầu</th></tr></thead><tbody>{items.map((item) => <tr key={`${item.serverId}-${item.container.id}`}><td><div className="flex items-center gap-2"><div className="grid size-8 shrink-0 place-items-center rounded-lg bg-slate-100"><Box className="size-4 text-slate-500" /></div><div><p className="font-semibold text-slate-950">{item.container.name}</p><p className="font-mono text-xs text-slate-400">{shortId(item.container.id)}</p></div></div></td><td><Badge value={item.container.origin} /></td><td><span className="capitalize">{item.container.state}</span>{item.container.jobId && <Link href={"/workloads/" + item.container.jobId} className="mt-1 block text-xs underline">Xem job</Link>}</td><td className="max-w-72 truncate font-mono text-xs">{item.container.image}</td><td><Link href={`/servers/${item.serverId}`} className="font-semibold hover:text-[var(--brand-red-dark)]">{item.serverName}</Link></td><td>{item.container.gpuUuids?.length ? <span className="font-mono text-xs">{item.container.gpuUuids.map((uuid) => shortId(uuid, 12)).join(", ")}</span> : <span className="inline-flex items-center gap-1 text-xs text-slate-400"><Info className="size-3.5" />Không báo cáo</span>}</td><td>{formatDateTime(item.container.startedAt)}</td></tr>)}</tbody></table></div>}
    </Card>
  </>;
}
