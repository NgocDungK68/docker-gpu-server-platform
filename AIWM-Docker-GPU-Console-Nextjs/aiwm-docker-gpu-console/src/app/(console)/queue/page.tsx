"use client";

import Link from "next/link";
import { useSession } from "@/features/identity/session";
import { profileLabel, workloadMessage } from "@/lib/utils/display";
import { CircleAlert, ListOrdered, Play, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useQueue, useRunScheduler } from "@/features/workloads/use-workloads";
import { relativeTime, shortId } from "@/lib/utils/format";

export default function QueuePage() {
  const { user } = useSession();
  const query = useQueue();
  const runMutation = useRunScheduler();
  const { pushToast } = useToast();
  const run = () => runMutation.mutate(undefined, { onSuccess: (result) => pushToast({ tone: "success", title: "Đã cập nhật cấp phát", description: result.assignedJobs ? `${result.assignedJobs} workload được cấp tài nguyên.` : "Chưa có tài nguyên phù hợp." }), onError: (error) => pushToast({ tone: "error", title: "Không cập nhật được cấp phát", description: error.message }) });
  const jobs = query.data ?? [];

  return <><PageHeader title="Hàng đợi" actions={<><Button variant="secondary" onClick={() => query.refetch()}><RefreshCw className="size-4" />Làm mới</Button>{user.role === "ADMIN" && <Button onClick={run} disabled={runMutation.isPending}><Play className="size-4" />{runMutation.isPending ? "Đang chạy…" : "Cập nhật cấp phát"}</Button>}</>} />
    <div className="mb-4 grid gap-3 md:grid-cols-3"><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Workload đang chờ</p><p className="mt-1 text-2xl font-bold text-slate-950">{jobs.length}</p></Card><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Đứng đầu hàng đợi</p><p className="mt-1 text-2xl font-bold text-[var(--brand-red-dark)]">{jobs[0]?.necessityLabel ?? "—"}</p></Card><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Chờ lâu nhất</p><p className="mt-1 text-lg font-bold text-slate-950">{jobs.length ? relativeTime([...jobs].sort((a, b) => a.createdAt.localeCompare(b.createdAt))[0].createdAt) : "—"}</p></Card></div>
    <div className="space-y-5"><Card><CardHeader title="Thứ tự xử lý" />{query.isLoading ? <TableSkeleton /> : query.error ? <div className="p-5"><ErrorState message={query.error.message} onRetry={query.refetch} /></div> : jobs.length === 0 ? <EmptyState icon={<ListOrdered className="size-6" />} title="Hàng đợi đang trống" description="Mọi job đã được xếp hoặc chưa có workload mới." action={<Link href="/workloads/new" className="btn btn-primary">Tạo workload</Link>} /> : <div className="divide-y divide-slate-100">{jobs.map((job, index) => <Link href={`/workloads/${job.id}`} key={job.id} className="grid gap-3 p-4 transition hover:bg-slate-50 sm:grid-cols-[46px_1fr_auto] sm:items-center"><div className="grid size-10 place-items-center rounded-xl bg-slate-950 text-sm font-bold text-white">#{index + 1}</div><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-bold text-slate-950">{job.name}</p><Badge value={job.status} /></div><p className="mt-1 truncate text-xs text-slate-500">{shortId(job.id, 18)} · {job.resources.gpuCount} × {profileLabel(job.resources.performanceProfile)} · chờ {relativeTime(job.createdAt)}</p>{job.statusReason && <p className="mt-2 flex items-start gap-1.5 text-xs leading-5 text-amber-700"><CircleAlert className="mt-0.5 size-3.5 shrink-0" />{workloadMessage(job.status, job.statusReason)}</p>}</div><div className="text-left sm:text-right"><p className="text-xs font-semibold text-slate-400">Tính cần thiết</p><p className="text-xl font-bold text-[var(--brand-red-dark)]">{job.necessityLabel}</p></div></Link>)}</div>}</Card>
    </div>
  </>;
}
