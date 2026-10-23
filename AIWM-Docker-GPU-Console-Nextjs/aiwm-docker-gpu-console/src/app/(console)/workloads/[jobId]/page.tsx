"use client";

import Link from "next/link";
import { ArrowLeft, Download, RotateCcw, Square, TriangleAlert } from "lucide-react";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { ErrorState, PageHeader, TableSkeleton } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useJob, useStopJob, useContinueJob, useWorkloadNow } from "@/features/workloads/use-workloads";
import { useServers } from "@/features/inventory/use-inventory";
import { JobLifecycle } from "@/components/workloads/job-lifecycle";
import { WorkloadStatus } from "@/components/workloads/allocation-status";
import { ContinuationDialog } from "@/components/workloads/continuation-dialog";
import { canContinue, checkpointLabel, terminationLabel, workloadGPU, workloadTypeLabel, workloadWarning } from "@/lib/jobs/presentation";
import { formatDateTime } from "@/lib/utils/format";
import { api } from "@/lib/api/api-client";

const stoppable = ["QUEUED", "ASSIGNED", "STARTING", "RUNNING"];

export default function WorkloadDetailPage() {
  const { jobId } = useParams<{ jobId: string }>();
  const router = useRouter();
  const query = useJob(jobId);
  const servers = useServers();
  const now = useWorkloadNow();
  const stopMutation = useStopJob(jobId);
  const continuation = useContinueJob(jobId);
  const download = useMutation({ mutationFn: () => api.artifactDownload(jobId) });
  const { pushToast } = useToast();
  const [stopOpen, setStopOpen] = useState(false);
  const [resumeOpen, setResumeOpen] = useState(false);
  const job = query.data;
  if (query.isLoading) return <><PageHeader title="Đang tải workload…" /><Card><TableSkeleton rows={8} /></Card></>;
  if (query.error || !job) return <><PageHeader title="Không tìm thấy workload" /><ErrorState message="Không tải được thông tin workload." onRetry={query.refetch} /></>;

  const reason = job.terminationReason === "COMPLETED" ? null : terminationLabel(job);
  const resume = canContinue(job);
  const modelReady = job.training?.artifactStatus === "READY";
  const stop = () => stopMutation.mutate(undefined, {
    onSuccess: () => { setStopOpen(false); pushToast({ tone: "success", title: "Đã gửi yêu cầu dừng" }); },
    onError: () => pushToast({ tone: "error", title: "Chưa dừng được workload", description: "Vui lòng thử lại." }),
  });
  const downloadModel = () => download.mutate(undefined, {
    onSuccess: result => {
      try {
        const url = new URL(result.url);
        if (!["http:", "https:"].includes(url.protocol)) throw new Error("Invalid download URL");
        window.location.assign(url.href);
      } catch { pushToast({ tone: "error", title: "Chưa tải được model", description: "Vui lòng thử lại." }); }
    },
  });

  return <>
    <Link href="/workloads" className="mb-4 inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-950"><ArrowLeft className="size-4" />Danh sách workloads</Link>
    <PageHeader title={job.name} actions={<>
      <WorkloadStatus job={job} now={now} />
      {stoppable.includes(job.status) && <Button variant="danger" onClick={() => setStopOpen(true)}><Square className="size-4" />{job.status === "QUEUED" || job.status === "ASSIGNED" ? "Hủy lịch" : "Dừng workload"}</Button>}
    </>} />
    {workloadWarning(job, now) && <div role="status" className="mb-5 flex items-center gap-3 rounded-xl border border-amber-200 bg-amber-50 px-5 py-4 text-sm font-semibold text-amber-800"><TriangleAlert className="size-5 shrink-0" />Thời gian cấp phát sắp hết</div>}
    <div className="grid gap-5 xl:grid-cols-[1.2fr_1fr]">
      <div className="space-y-5">
        <Card><CardHeader title="Thông tin workload" /><dl className="divide-y divide-slate-100 px-5">
          <Spec label="Dự án">{job.name}</Spec>
          <Spec label="Loại workload">{workloadTypeLabel(job)}</Spec>
          <Spec label="GPU">{workloadGPU(job, servers.data)}</Spec>
          {reason && <Spec label="Kết quả">{reason}</Spec>}
        </dl>{servers.isError && <p role="status" className="px-5 pb-4 text-sm text-amber-700">Chưa tải được thông tin model GPU. <button className="underline" onClick={() => void servers.refetch()}>Thử lại</button></p>}</Card>
        <JobLifecycle job={job} now={now} />
      </div>
      {job.workloadType === "TRAINING" && <div className="space-y-5">
        <Card><CardHeader title="Checkpoint gần nhất" /><div className="space-y-4 p-5 text-sm">
          <p className="font-semibold">{checkpointLabel(job)}</p>
          {job.training?.latestCheckpointURI && <dl className="space-y-3">
            <div className="flex justify-between gap-4"><dt className="text-slate-500">Lưu lúc</dt><dd className="font-semibold">{formatDateTime(job.training.checkpointCreatedAt)}</dd></div>
            {job.training.checkpointStep !== undefined && <div className="flex justify-between gap-4"><dt className="text-slate-500">Bước đã lưu</dt><dd className="font-semibold">{job.training.checkpointStep.toLocaleString("vi-VN")}</dd></div>}
          </dl>}
          {resume && <div className="rounded-xl bg-blue-50 p-4"><p className="mb-3 font-semibold text-blue-900">Có thể tiếp tục</p><Button onClick={() => { continuation.reset(); setResumeOpen(true); }}><RotateCcw className="size-4" />Xin cấp phát tiếp</Button></div>}
        </div></Card>
        <Card><CardHeader title="Model đầu ra" /><div className="space-y-4 p-5 text-sm">
          <p className={modelReady ? "font-semibold text-emerald-700" : "font-medium text-slate-600"}>{modelReady ? "Model đã sẵn sàng" : job.training?.artifactStatus === "FAILED" ? "Không lưu được model đầu ra" : job.training?.artifactStatus === "SAVING" || job.status === "SUCCEEDED" ? "Đang lưu model" : "Chưa có model đầu ra"}</p>
          {modelReady && <><p className="text-slate-500">{formatDateTime(job.training?.artifactCreatedAt)}</p><Button onClick={downloadModel} disabled={download.isPending}><Download className="size-4" />{download.isPending ? "Đang chuẩn bị…" : "Tải model"}</Button></>}
          {download.isError && <p role="alert" className="text-red-700">Chưa tải được model. Vui lòng thử lại.</p>}
        </div></Card>
      </div>}
    </div>
    <Dialog open={stopOpen} onClose={() => setStopOpen(false)} onConfirm={stop} busy={stopMutation.isPending} destructive title="Dừng workload này?" description="Hủy lịch chưa chạy hoặc dừng workload đang thực thi. Bạn có muốn tiếp tục?" confirmLabel="Dừng workload" />
    {resumeOpen && <ContinuationDialog busy={continuation.isPending} error={continuation.isError ? "Chưa gửi được yêu cầu. Kiểm tra thời gian, tài khoản và thử lại." : null} onClose={() => setResumeOpen(false)} onSubmit={input => continuation.mutate(input, {
      onSuccess: next => { setResumeOpen(false); pushToast({ tone: "success", title: "Đã gửi yêu cầu cấp phát" }); router.push("/workloads/" + next.id); },
    })} />}
  </>;
}

function Spec({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="flex items-start justify-between gap-5 py-3.5 text-sm"><dt className="shrink-0 text-slate-500">{label}</dt><dd className="min-w-0 text-right font-semibold text-slate-900">{children}</dd></div>;
}
