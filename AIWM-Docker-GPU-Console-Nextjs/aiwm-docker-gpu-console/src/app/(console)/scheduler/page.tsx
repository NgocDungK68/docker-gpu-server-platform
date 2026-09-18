"use client";

import { useSession } from "@/features/identity/session";
import { Play, Cpu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { PageHeader, ErrorState, TableSkeleton, EmptyState } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useJobs, useRunScheduler } from "@/features/workloads/use-workloads";

const strategies = [
  ["First Fit", "Thứ tự server ID cố định; chọn candidate đầu tiên."],
  ["Best Fit", "Giảm số GPU phù hợp còn dư sau placement."],
  ["Bin Packing", "Gom workload vào server đã dùng, giảm số GPU còn dư."],
  ["Fragmentation-aware", "Ưu tiên host đang dùng, lấp kín host; giữ host trống cho job lớn."],
];

export default function SchedulerPage() {
  const { user } = useSession();
  const mutation = useRunScheduler();
  const jobs = useJobs();
  const { pushToast } = useToast();
  const run = () => mutation.mutate(undefined, {
    onSuccess: (result) => pushToast({ tone: "success", title: "Đã chạy scheduler", description: result.assignedJobs + " job được cấp GPU." }),
    onError: (error) => pushToast({ tone: "error", title: "Scheduler gặp lỗi", description: error.message }),
  });
  const assigned = jobs.data?.filter((job) => job.assignment) ?? [];
  return <><PageHeader title="Scheduler" description="Quyết định cấp phát từ Control Plane. Strategy do cấu hình CP quyết định, luôn giữ ownership đơn vị."
    actions={user.role === "ADMIN" ? <Button onClick={run} disabled={mutation.isPending}><Play className="size-4" />Chạy một chu kỳ</Button> : undefined} />
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">{strategies.map(([name, description]) => <Card key={name} className="p-5">
      <h2 className="font-bold text-slate-950">{name}</h2><p className="mt-3 text-sm leading-6 text-slate-600">{description}</p>
    </Card>)}</div>
    <Card className="mt-5"><CardHeader title="Quyết định gần đây" description="Kết quả placement thực của các job đã được gán GPU" />
      {jobs.isLoading ? <TableSkeleton rows={4} /> : jobs.error ? <ErrorState message={jobs.error.message} /> : assigned.length === 0 ? <EmptyState icon={<Cpu />} title="Chưa có placement" description="Tạo một workload để xem kết quả cấp phát và reservation." /> :
        <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Job</th><th>Strategy</th><th>Reservation</th><th>Giải thích</th></tr></thead>
          <tbody>{assigned.slice(0, 20).map((job) => <tr key={job.id}><td>{job.name}</td><td>{job.assignment?.strategy}</td><td><Badge value={job.assignment?.reservationState ?? ""} /></td><td className="max-w-lg">Server: {job.assignment?.serverId}</td></tr>)}</tbody>
        </table></div>}
    </Card>
  </>;
}
