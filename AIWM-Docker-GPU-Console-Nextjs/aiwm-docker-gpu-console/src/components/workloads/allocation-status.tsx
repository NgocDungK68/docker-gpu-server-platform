import type { Job } from "@/lib/api/types";
import { allocationProgress, allocationWindow, workloadDisplay } from "@/lib/jobs/presentation";
import { formatDateTime } from "@/lib/utils/format";

const progressColors = { success: "bg-emerald-500", warning: "bg-amber-400", danger: "bg-rose-500" };
const badgeColors = { success: "badge-success", warning: "badge-warning", danger: "badge-danger", info: "badge-info", neutral: "badge-neutral" };

export function AllocationProgress({ job, now }: { job: Job; now: number | null }) {
  const progress = allocationProgress(job, now);
  if (!progress) return <span className="text-sm text-slate-400">—</span>;
  return <div className="min-w-32 space-y-1.5" data-tone={progress.tone}>
    <div role="progressbar" aria-label={"Tiến độ cấp phát " + job.name} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress.percent} className="h-2 overflow-hidden rounded-full bg-slate-100">
      <div className={"h-full rounded-full transition-[width] " + progressColors[progress.tone]} style={{ width: progress.percent + "%" }} />
    </div>
    <p className="text-right text-xs font-semibold tabular-nums text-slate-500">{progress.percent}%</p>
  </div>;
}

export function WorkloadStatus({ job, now }: { job: Job; now: number | null }) {
  const display = workloadDisplay(job, now);
  return <span className={"badge " + badgeColors[display.tone]}>{display.label}</span>;
}

export function allocationTooltip(job: Job): string | undefined {
  const window = allocationWindow(job);
  return window ? formatDateTime(new Date(window.start).toISOString()) + " → " + formatDateTime(new Date(window.end).toISOString()) : undefined;
}
