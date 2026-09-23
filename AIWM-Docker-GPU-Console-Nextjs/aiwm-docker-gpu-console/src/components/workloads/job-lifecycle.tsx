import { Card, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatDateTime } from "@/lib/utils/format";
import type { Job } from "@/lib/api/types";

export function JobLifecycle({ job }: { job: Job }) {
  const planning = job.status === "RUNNING" ? "Đang chạy" : job.status === "STARTING" ? "Đang khởi động" : job.status === "ASSIGNED" ? "Đã giữ tài nguyên · Chưa chạy" : job.status === "QUEUED" ? "Chưa được cấp tài nguyên" : job.status === "STOPPING" ? "Đang dừng" : "Đã kết thúc";
  return <Card><CardHeader title="Lịch cấp phát" />
    <div className="space-y-4 p-5 text-sm">
      <p className="font-semibold">{planning}</p>
      {job.neededAt && <div className="space-y-1 text-slate-600"><p>Bắt đầu yêu cầu: {formatDateTime(job.requestedStartAt)}</p><p>Thời lượng sử dụng: {(job.ttlSeconds ?? 0) / 60} phút</p><p>Kết thúc dự kiến: {formatDateTime(job.requestedEndAt)}</p></div>}
      {job.assignment && <div className="rounded-lg bg-slate-50 p-3">
        <p className="font-semibold">{job.assignment.reservationState === "RELEASED" ? "Đã trả tài nguyên" : "Đã giữ tài nguyên"}</p>
        <p className="mt-1">{formatDateTime(job.assignment.startAt)} → {formatDateTime(job.assignment.endAt)}</p>
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
