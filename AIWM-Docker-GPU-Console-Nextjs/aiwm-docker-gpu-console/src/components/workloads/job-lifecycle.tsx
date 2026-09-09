import { Card, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatDateTime } from "@/lib/utils/format";
import type { Job } from "@/lib/api/types";

export function JobLifecycle({ job }: { job: Job }) {
  return <Card><CardHeader title="Vòng đời và reservation" />
    <div className="space-y-4 p-5 text-sm">
      {job.assignment && <div className="rounded-lg bg-slate-50 p-3">
        <p className="font-semibold">{job.assignment.reservationState} · score {job.assignment.score}</p>
        <p className="mt-1 text-slate-600">{job.assignment.reason}</p>
        {job.assignment.releasedAt && <p className="mt-1 text-xs text-slate-500">Nhả GPU: {formatDateTime(job.assignment.releasedAt)}</p>}
      </div>}
      <p className="break-all text-xs text-slate-500">Container: {job.containerId || "Chưa quan sát được"}</p>
      <ol className="space-y-4">{job.events?.map((event, index) => <li key={event.at + index} className="border-l-2 border-slate-200 pl-3">
        <Badge value={event.status} /><p className="mt-1 text-slate-600">{event.reason}</p>
        <time className="text-xs text-slate-400">{formatDateTime(event.at)}</time>
      </li>)}</ol>
    </div>
  </Card>;
}
