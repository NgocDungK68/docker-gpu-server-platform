"use client";

import Link from "next/link";
import { Boxes, ChevronRight, Plus, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useJobs } from "@/features/workloads/use-workloads";
import type { JobStatus } from "@/lib/api/types";
import { relativeTime, shortId } from "@/lib/utils/format";

const filters: Array<{ value: "ALL" | JobStatus; label: string }> = [{ value: "ALL", label: "Mọi trạng thái" }, { value: "RUNNING", label: "Đang chạy" }, { value: "QUEUED", label: "Đang chờ" }, { value: "STARTING", label: "Đang khởi động" }, { value: "FAILED", label: "Thất bại" }, { value: "STOPPED", label: "Đã dừng" }];

export default function WorkloadsPage() {
  const query = useJobs();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<"ALL" | JobStatus>("ALL");
  const jobs = useMemo(() => (query.data ?? []).filter((job) => (status === "ALL" || job.status === status) && `${job.name} ${job.id} ${job.image}`.toLowerCase().includes(search.toLowerCase())).sort((a, b) => b.createdAt.localeCompare(a.createdAt)), [query.data, search, status]);

  return <><PageHeader eyebrow="Managed execution" title="AI workloads" description="Các job Docker do AIWM quản lý. Container có sẵn trên server được theo dõi riêng và không xuất hiện như managed workload." actions={<Link href="/workloads/new" className="btn btn-primary"><Plus className="size-4" />Tạo workload</Link>} />
    <Card><div className="flex flex-col gap-3 border-b border-slate-100 p-4 md:flex-row"><div className="relative flex-1"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-400" /><input className="field-input pl-9" placeholder="Tìm tên workload, ID hoặc image…" value={search} onChange={(event) => setSearch(event.target.value)} /></div><select className="field-select md:w-56" value={status} onChange={(event) => setStatus(event.target.value as "ALL" | JobStatus)}>{filters.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></div>
      {query.isLoading ? <TableSkeleton rows={7} /> : query.error ? <div className="p-5"><ErrorState message={query.error.message} onRetry={query.refetch} /></div> : jobs.length === 0 ? <EmptyState icon={<Boxes className="size-6" />} title="Chưa có workload" description="Tạo job Docker đầu tiên hoặc thay đổi bộ lọc hiện tại." action={<Link href="/workloads/new" className="btn btn-primary">Tạo workload</Link>} /> : <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Workload</th><th>Trạng thái</th><th>Yêu cầu</th><th>Tính cần thiết</th><th>Placement</th><th>Cập nhật</th><th /></tr></thead><tbody>{jobs.map((job) => <tr key={job.id}><td><Link href={`/workloads/${job.id}`} className="font-bold text-slate-950 hover:text-[var(--brand-red-dark)]">{job.name}</Link><p className="mt-1 font-mono text-xs text-slate-400">{shortId(job.id, 18)}</p></td><td><Badge value={job.status} /></td><td><p className="font-semibold">{job.resources.gpuCount} × {job.resources.performanceProfile || job.resources.gpuModel || "GPU bất kỳ"}</p><p className="mt-0.5 text-xs text-slate-400">VRAM tối thiểu {job.resources.minVramMiB ?? 0} MiB/GPU</p></td><td><span className="rounded-lg bg-slate-100 px-2.5 py-1 font-bold text-slate-700">{job.necessityLabel}</span></td><td>{job.assignment ? <><p className="font-mono text-xs text-slate-700">{shortId(job.assignment.serverId, 16)}</p><p className="mt-0.5 text-xs text-slate-400">{job.assignment.gpuUuids.length} GPU</p></> : <span className="text-slate-400">Chưa xếp</span>}</td><td>{relativeTime(job.updatedAt)}</td><td><Link href={`/workloads/${job.id}`} aria-label={`Xem ${job.name}`} className="grid size-8 place-items-center rounded-lg text-slate-400 hover:bg-red-50 hover:text-[var(--brand-red)]"><ChevronRight className="size-4" /></Link></td></tr>)}</tbody></table></div>}
    </Card>
  </>;
}
