"use client";

import Link from "next/link";
import { Boxes, Plus, RefreshCw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { AllocationProgress, WorkloadStatus, allocationTooltip } from "@/components/workloads/allocation-status";
import { useJobs, useWorkloadNow } from "@/features/workloads/use-workloads";
import { useServers } from "@/features/inventory/use-inventory";
import { allocationTimeLabel, workloadDisplay, workloadGPU, workloadTypeLabel } from "@/lib/jobs/presentation";

const filters = [
  ["ALL", "Mọi trạng thái"], ["waiting", "Đang chờ"], ["planned", "Đã lên lịch"],
  ["running", "Đang chạy"], ["warning", "Cảnh báo"], ["saving-checkpoint", "Đang lưu checkpoint"],
  ["resumable", "Có thể tiếp tục"], ["expired", "Hết thời gian"], ["completed", "Hoàn thành"],
  ["saving-model", "Đang lưu model"], ["output-failed", "Lỗi lưu model"], ["failed", "Thất bại"],
  ["starting", "Đang khởi động"], ["stopping", "Đang dừng / thu hồi"], ["stopped", "Đã dừng"], ["cancelled", "Đã hủy"],
];

export default function WorkloadsPage() {
  const query = useJobs();
  const servers = useServers();
  const now = useWorkloadNow();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("ALL");
  const jobs = useMemo(() => (query.data ?? [])
    .filter(job => (status === "ALL" || workloadDisplay(job, now).key === status) && job.name.toLocaleLowerCase("vi").includes(search.toLocaleLowerCase("vi")))
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt)), [query.data, search, status, now]);

  return <>
    <PageHeader title="Workloads" actions={<>
      <Button variant="secondary" disabled={query.isFetching} onClick={() => { void query.refetch(); void servers.refetch(); }}><RefreshCw className="size-4" />Làm mới</Button>
      <Link href="/workloads/new" className="btn btn-primary"><Plus className="size-4" />Tạo workload</Link>
    </>} />
    <Card>
      <div className="flex flex-col gap-3 border-b border-slate-100 p-4 md:flex-row">
        <div className="relative min-w-0 flex-1"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" /><input aria-label="Tìm dự án" className="field-input pl-9" placeholder="Tìm tên dự án…" value={search} onChange={event => setSearch(event.target.value)} /></div>
        <div className="shrink-0 md:w-56"><select aria-label="Lọc trạng thái" className="field-select" value={status} onChange={event => setStatus(event.target.value)}>{filters.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
      </div>
      {query.isLoading ? <TableSkeleton rows={7} /> : query.error ? <div className="p-5"><ErrorState message="Không tải được danh sách workload." onRetry={query.refetch} /></div> : jobs.length === 0 ?
        <EmptyState icon={<Boxes className="size-6" />} title="Chưa có workload" description="Tạo workload hoặc thay đổi bộ lọc hiện tại." action={<Link href="/workloads/new" className="btn btn-primary">Tạo workload</Link>} /> :
        <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Dự án</th><th>Loại workload</th><th>GPU</th><th>Thời gian</th><th>Tiến độ cấp phát</th><th>Trạng thái</th></tr></thead>
          <tbody>{jobs.map(job => <tr key={job.id}>
            <td><Link href={"/workloads/" + job.id} className="font-bold text-slate-950 hover:text-[var(--brand-red-dark)]">{job.name}</Link></td>
            <td>{workloadTypeLabel(job)}</td>
            <td><span className="font-semibold">{workloadGPU(job, servers.data)}</span></td>
            <td title={allocationTooltip(job)} className="text-sm">{allocationTimeLabel(job, now)}</td>
            <td><AllocationProgress job={job} now={now} /></td>
            <td><WorkloadStatus job={job} now={now} /></td>
          </tr>)}</tbody></table></div>}
      {servers.isError && query.data?.length ? <div role="status" className="border-t border-slate-100 px-5 py-3 text-sm text-amber-700">Chưa tải được thông tin model GPU. <button className="font-semibold underline" onClick={() => void servers.refetch()}>Thử lại</button></div> : null}
    </Card>
  </>;
}
