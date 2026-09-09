"use client";

import Link from "next/link";
import { Cpu, Search, SlidersHorizontal } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { Progress } from "@/components/ui/progress";
import { useGPUs } from "@/features/inventory/use-inventory";
import type { GPUState } from "@/lib/api/types";
import { formatMiB, formatPercent, shortId } from "@/lib/utils/format";

const states: Array<{ value: "ALL" | GPUState; label: string }> = [
  { value: "ALL", label: "Tất cả trạng thái" }, { value: "FREE", label: "Sẵn sàng" },
  { value: "ALLOCATED", label: "AIWM đang dùng" }, { value: "OCCUPIED_LEGACY", label: "Workload cũ" },
  { value: "OCCUPIED_UNKNOWN", label: "Consumer chưa rõ" }, { value: "UNHEALTHY", label: "Không khoẻ" },
];

export default function GPUsPage() {
  const query = useGPUs();
  const [search, setSearch] = useState("");
  const [state, setState] = useState<"ALL" | GPUState>("ALL");
  const items = useMemo(() => (query.data ?? []).filter((item) => (state === "ALL" || item.gpu.state === state) && `${item.gpu.model} ${item.gpu.uuid} ${item.serverName}`.toLowerCase().includes(search.toLowerCase())), [query.data, search, state]);
  const free = (query.data ?? []).filter((item) => item.gpu.state === "FREE").length;

  return <><PageHeader eyebrow="Physical inventory" title="GPU inventory" description="GPU được xác định theo UUID thật. Legacy hoặc process chưa rõ nguồn luôn bị loại khỏi tập schedulable." />
    <div className="mb-4 grid gap-3 sm:grid-cols-3"><Card className="p-4"><p className="text-xs font-semibold uppercase tracking-wider text-slate-400">Tổng GPU</p><p className="mt-1 text-2xl font-bold text-slate-950">{query.data?.length ?? 0}</p></Card><Card className="p-4"><p className="text-xs font-semibold uppercase tracking-wider text-slate-400">Sẵn sàng</p><p className="mt-1 text-2xl font-bold text-emerald-600">{free}</p></Card><Card className="p-4"><p className="text-xs font-semibold uppercase tracking-wider text-slate-400">Được bảo vệ</p><p className="mt-1 text-2xl font-bold text-violet-700">{(query.data ?? []).filter((item) => ["OCCUPIED_LEGACY", "OCCUPIED_UNKNOWN"].includes(item.gpu.state)).length}</p></Card></div>
    <Card><div className="flex flex-col gap-3 border-b border-slate-100 p-4 md:flex-row"><div className="relative flex-1"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" /><input className="field-input pl-9" placeholder="Tìm GPU UUID, model hoặc server…" value={search} onChange={(event) => setSearch(event.target.value)} /></div><div className="relative md:w-64"><SlidersHorizontal className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" /><select className="field-select pl-9" value={state} onChange={(event) => setState(event.target.value as "ALL" | GPUState)}>{states.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}</select></div></div>
      {query.isLoading ? <TableSkeleton rows={7} /> : query.error ? <div className="p-5"><ErrorState message={query.error.message} onRetry={query.refetch} /></div> : items.length === 0 ? <EmptyState icon={<Cpu className="size-6" />} title="Không có GPU phù hợp" description="Thay đổi bộ lọc hoặc kiểm tra Agent inventory." /> : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>GPU</th><th>Máy chủ</th><th>Trạng thái</th><th>VRAM</th><th>GPU util</th><th>Nhiệt độ</th></tr></thead><tbody>{items.map((item) => { const memoryPct = item.gpu.memoryTotalMiB ? (item.gpu.memoryUsedMiB / item.gpu.memoryTotalMiB) * 100 : 0; return <tr key={`${item.serverId}-${item.gpu.uuid}`}><td><p className="font-semibold text-slate-950">GPU {item.gpu.index} · {item.gpu.model}</p><p className="mt-0.5 font-mono text-xs text-slate-400">{shortId(item.gpu.uuid, 20)}</p></td><td><Link href={`/servers/${item.serverId}`} className="font-semibold text-slate-700 hover:text-[var(--brand-red-dark)]">{item.serverName}</Link><div className="mt-1"><Badge value={item.serverStatus} /></div></td><td><Badge value={item.gpu.state} /></td><td className="min-w-44"><div className="mb-1 flex justify-between text-xs"><span>{formatMiB(item.gpu.memoryUsedMiB)}</span><span className="text-slate-400">/ {formatMiB(item.gpu.memoryTotalMiB)}</span></div><Progress value={memoryPct} tone={memoryPct > 85 ? "red" : "blue"} /></td><td className="font-semibold">{formatPercent(item.gpu.utilizationPct)}</td><td>{item.gpu.temperatureC ? `${item.gpu.temperatureC}°C` : "—"}</td></tr>; })}</tbody></table></div>}
    </Card>
  </>;
}
