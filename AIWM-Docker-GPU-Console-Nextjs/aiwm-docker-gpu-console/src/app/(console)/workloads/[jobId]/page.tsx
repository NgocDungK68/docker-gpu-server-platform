"use client";

import Link from "next/link";
import { profileLabel, workloadMessage } from "@/lib/utils/display";
import { ArrowLeft, Clock3, Cpu, Server, Square } from "lucide-react";
import { useParams } from "next/navigation";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useJob, useStopJob } from "@/features/workloads/use-workloads";
import { JobLifecycle } from "@/components/workloads/job-lifecycle";
import { formatDateTime, formatMiB, shortId } from "@/lib/utils/format";

const stoppable = ["QUEUED", "ASSIGNED", "STARTING", "RUNNING"];

export default function WorkloadDetailPage() {
  const { jobId } = useParams<{ jobId: string }>();
  const query = useJob(jobId);
  const stopMutation = useStopJob(jobId);
  const { pushToast } = useToast();
  const [stopOpen, setStopOpen] = useState(false);
  const job = query.data;
  if (query.isLoading) return <><PageHeader title="Đang tải workload…" /><Card><TableSkeleton rows={8} /></Card></>;
  if (query.error || !job) return <><PageHeader title="Không tìm thấy workload" /><ErrorState message={query.error?.message} /></>;

  const stop = () => stopMutation.mutate(undefined, { onSuccess: () => { setStopOpen(false); pushToast({ tone: "success", title: "Đã gửi lệnh dừng" }); }, onError: (error) => pushToast({ tone: "error", title: "Không dừng được workload", description: error.message }) });
  return <><Link href="/workloads" className="mb-4 inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-950"><ArrowLeft className="size-4" />Danh sách workloads</Link><PageHeader  title={job.name} description={job.id} actions={<><Badge value={job.status} label={job.status === "ASSIGNED" ? "Đã lên lịch · Chưa chạy" : undefined} />{stoppable.includes(job.status) && <Button variant="danger" onClick={() => setStopOpen(true)}><Square className="size-4" />{(job.status === "QUEUED" || job.status === "ASSIGNED") ? "Hủy lịch" : "Dừng workload"}</Button>}</>} />
    <div className="grid gap-5 xl:grid-cols-[1.25fr_.75fr]">
      <div className="space-y-5"><Card><CardHeader title="Trạng thái điều phối" description="Lịch đăng ký và trạng thái thực thi hiện tại" /><div className="p-5"><div className="flex flex-wrap items-center gap-3"><Badge value={job.status} label={job.status === "ASSIGNED" ? "Đã lên lịch · Chưa chạy" : undefined} /><p className="text-sm font-medium text-slate-700">{workloadMessage(job.status, job.statusReason)}</p></div><div className="mt-5 grid gap-3 sm:grid-cols-3"><InfoTile icon={<Cpu className="size-5" />} label="Nhu cầu GPU" value={`${job.resources.gpuCount} × ${profileLabel(job.resources.performanceProfile)}`} note={`VRAM ≥ ${formatMiB(job.resources.minVramMiB ?? 0)}`} /><InfoTile icon={<Server className="size-5" />} label="Server" value={job.assignment ? shortId(job.assignment.serverId, 18) : "Chưa xếp"} note={job.assignment ? formatDateTime(job.assignment.assignedAt) : "Đang ở hàng đợi"} /><InfoTile icon={<Clock3 className="size-5" />} label="Tính cần thiết" value={job.necessityLabel} note="" /></div></div></Card>
      <Card><CardHeader title="Cấu hình workload" /><dl className="divide-y divide-slate-100 px-5"><Spec label="Image"><code className="break-all font-mono text-xs">{job.image}</code></Spec><Spec label="Command"><code className="break-all font-mono text-xs">{job.command?.join(" ") || "Mặc định của image"}</code></Spec><Spec label="CPU / RAM"><span>{job.resources.cpuMilli ?? 0}m / {formatMiB(job.resources.memoryMiB ?? 0)}</span></Spec></dl></Card>
      {job.assignment && <Card><CardHeader title="Tài nguyên được cấp" /><div className="p-5"><Link href={`/servers/${job.assignment.serverId}`} className="mb-3 inline-flex items-center gap-2 font-bold text-slate-900 hover:text-[var(--brand-red-dark)]"><Server className="size-4" />{job.assignment.serverId}</Link><div className="grid gap-2 sm:grid-cols-2">{job.assignment.gpuUuids.map((uuid) => <div key={uuid} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 font-mono text-xs text-slate-700">{uuid}</div>)}</div></div></Card>}
      </div>
      <div className="space-y-5"><Card><CardHeader title="Thông tin workload" /><dl className="divide-y divide-slate-100 px-5"><Spec label="Tạo lúc"><span>{formatDateTime(job.createdAt)}</span></Spec><Spec label="Cập nhật"><span>{formatDateTime(job.updatedAt)}</span></Spec><Spec label="Environment"><span>{Object.keys(job.environment ?? {}).length} biến</span></Spec></dl></Card>
      <JobLifecycle job={job} />
      </div>
    </div><Dialog open={stopOpen} onClose={() => setStopOpen(false)} onConfirm={stop} busy={stopMutation.isPending} destructive title="Dừng workload này?" description="Hủy lịch chưa chạy hoặc dừng workload đang thực thi. Bạn có muốn tiếp tục?" confirmLabel="Dừng workload" /></>;
}

function InfoTile({ icon, label, value, note }: { icon: React.ReactNode; label: string; value: string; note: string }) { return <div className="rounded-xl bg-slate-50 p-4"><div className="mb-3 text-slate-400">{icon}</div><p className="text-xs font-semibold text-slate-500">{label}</p><p className="mt-1 font-bold text-slate-950">{value}</p><p className="mt-1 text-xs text-slate-400">{note}</p></div>; }
function Spec({ label, children }: { label: string; children: React.ReactNode }) { return <div className="flex items-start justify-between gap-5 py-3.5 text-sm"><dt className="shrink-0 text-slate-500">{label}</dt><dd className="min-w-0 text-right font-semibold text-slate-900">{children}</dd></div>; }
