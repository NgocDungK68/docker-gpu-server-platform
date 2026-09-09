"use client";

import Link from "next/link";
import { CircleAlert, ListOrdered, Play, RefreshCw } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { EmptyState, ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useQueue, useRunScheduler } from "@/features/workloads/use-workloads";
import { relativeTime, shortId } from "@/lib/utils/format";

export default function QueuePage() {
  const query = useQueue();
  const runMutation = useRunScheduler();
  const { pushToast } = useToast();
  const run = () => runMutation.mutate(undefined, { onSuccess: (result) => pushToast({ tone: "success", title: "Đã chạy một chu kỳ scheduler", description: result.assignedJobs ? `${result.assignedJobs} job được gán tài nguyên.` : "Chưa có job nào tìm được placement phù hợp." }), onError: (error) => pushToast({ tone: "error", title: "Scheduler gặp lỗi", description: error.message }) });
  const jobs = query.data ?? [];

  return <><PageHeader eyebrow="Priority + FIFO" title="Hàng đợi điều phối" description="Backend hiện sắp thứ tự theo priority giảm dần, sau đó FIFO khi cùng priority." actions={<><Button variant="secondary" onClick={() => query.refetch()}><RefreshCw className="size-4" />Làm mới</Button><Button onClick={run} disabled={runMutation.isPending}><Play className="size-4" />{runMutation.isPending ? "Đang chạy…" : "Run once"}</Button></>} />
    <div className="mb-4 grid gap-3 md:grid-cols-3"><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Độ dài queue</p><p className="mt-1 text-2xl font-bold text-slate-950">{jobs.length}</p></Card><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Priority cao nhất</p><p className="mt-1 text-2xl font-bold text-[var(--brand-red-dark)]">{jobs[0]?.priority ?? "—"}</p></Card><Card className="p-4"><p className="text-xs font-bold uppercase tracking-wider text-slate-400">Chờ lâu nhất</p><p className="mt-1 text-lg font-bold text-slate-950">{jobs.length ? relativeTime([...jobs].sort((a, b) => a.createdAt.localeCompare(b.createdAt))[0].createdAt) : "—"}</p></Card></div>
    <div className="grid gap-5 xl:grid-cols-[1fr_320px]"><Card><CardHeader title="Thứ tự xử lý" description="Run once đi từ trên xuống và giữ job trong queue nếu không đủ GPU phù hợp" />{query.isLoading ? <TableSkeleton /> : query.error ? <div className="p-5"><ErrorState message={query.error.message} onRetry={query.refetch} /></div> : jobs.length === 0 ? <EmptyState icon={<ListOrdered className="size-6" />} title="Hàng đợi đang trống" description="Mọi job đã được xếp hoặc chưa có workload mới." action={<Link href="/workloads/new" className="btn btn-primary">Tạo workload</Link>} /> : <div className="divide-y divide-slate-100">{jobs.map((job, index) => <Link href={`/workloads/${job.id}`} key={job.id} className="grid gap-3 p-4 transition hover:bg-slate-50 sm:grid-cols-[46px_1fr_auto] sm:items-center"><div className="grid size-10 place-items-center rounded-xl bg-slate-950 text-sm font-bold text-white">#{index + 1}</div><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="font-bold text-slate-950">{job.name}</p><Badge value={job.status} /></div><p className="mt-1 truncate text-xs text-slate-500">{shortId(job.id, 18)} · {job.resources.gpuCount} × {job.resources.gpuModel || "GPU bất kỳ"} · chờ {relativeTime(job.createdAt)}</p>{job.statusReason && <p className="mt-2 flex items-start gap-1.5 text-xs leading-5 text-amber-700"><CircleAlert className="mt-0.5 size-3.5 shrink-0" />{job.statusReason}</p>}</div><div className="text-left sm:text-right"><p className="text-xs font-semibold text-slate-400">Priority</p><p className="text-xl font-bold text-[var(--brand-red-dark)]">{job.priority}</p></div></Link>)}</div>}</Card>
    <div className="space-y-4"><Card><CardHeader title="Logic hiện tại" /><div className="space-y-4 p-5 text-sm"><Rule number="1" title="Sắp queue" text="Priority cao hơn trước; cùng điểm thì job gửi sớm hơn trước." /><Rule number="2" title="Filter an toàn" text="Loại server offline/drain và GPU legacy/unknown/unhealthy." /><Rule number="3" title="Placement" text="Chọn First Fit, Best Fit, Bin Pack hoặc Fragmentation-aware theo từng job." /><Rule number="4" title="Commit nguyên tử" text="Reserve toàn bộ GPU UUID trước khi gửi command cho Agent." /></div></Card><div className="rounded-xl border border-sky-200 bg-sky-50 p-4 text-sm leading-6 text-sky-900">Job chờ giữ nguyên <b>priority</b>. Khi tài nguyên được giải phóng, scheduler tự thử lại; xem lý do chờ trong từng job.</div></div></div>
  </>;
}

function Rule({ number, title, text }: { number: string; title: string; text: string }) { return <div className="flex gap-3"><span className="grid size-7 shrink-0 place-items-center rounded-full bg-red-50 text-xs font-bold text-[var(--brand-red-dark)]">{number}</span><div><p className="font-bold text-slate-900">{title}</p><p className="mt-0.5 text-xs leading-5 text-slate-500">{text}</p></div></div>; }
