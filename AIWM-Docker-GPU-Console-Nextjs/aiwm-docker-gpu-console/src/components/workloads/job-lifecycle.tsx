import { Card, CardHeader } from "@/components/ui/card";
import { AllocationProgress } from "@/components/workloads/allocation-status";
import { allocationTimeLabel, allocationWindow } from "@/lib/jobs/presentation";
import { formatDateTime } from "@/lib/utils/format";
import type { Job } from "@/lib/api/types";

export function JobLifecycle({ job, now }: { job: Job; now: number | null }) {
  const window = allocationWindow(job);
  return <Card><CardHeader title="Lịch cấp phát" />
    <div className="space-y-5 p-5 text-sm">
      <dl className="grid gap-4 sm:grid-cols-2">
        <div><dt className="text-slate-500">Bắt đầu</dt><dd className="mt-1 font-semibold">{window ? formatDateTime(new Date(window.start).toISOString()) : "—"}</dd></div>
        <div><dt className="text-slate-500">Kết thúc</dt><dd className="mt-1 font-semibold">{window ? formatDateTime(new Date(window.end).toISOString()) : "—"}</dd></div>
      </dl>
      <p className="font-semibold text-slate-700">{allocationTimeLabel(job, now)}</p>
      <div><h3 className="mb-2 text-sm font-semibold text-slate-500">Tiến độ cấp phát</h3><AllocationProgress job={job} now={now} /></div>
    </div>
  </Card>;
}
