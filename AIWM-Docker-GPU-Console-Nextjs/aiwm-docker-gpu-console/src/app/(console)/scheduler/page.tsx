"use client";

import Link from "next/link";
import { useSession } from "@/features/identity/session";
import { Play, Cpu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { PageHeader, ErrorState, TableSkeleton, EmptyState } from "@/components/ui/page";
import { useToast } from "@/components/ui/toast";
import { useJobs, useRunScheduler } from "@/features/workloads/use-workloads";
import { formatDateTime } from "@/lib/utils/format";

export default function SchedulerPage() {
  const { user } = useSession();
  const mutation = useRunScheduler();
  const jobs = useJobs();
  const { pushToast } = useToast();
  const run = () => mutation.mutate(undefined, {
    onSuccess: (result) => pushToast({ tone: "success", title: "Đã cập nhật cấp phát", description: result.assignedJobs + " workload được cấp tài nguyên." }),
    onError: (error) => pushToast({ tone: "error", title: "Không cập nhật được cấp phát", description: error.message }),
  });
  const assigned = jobs.data?.filter(job => job.assignment) ?? [];
  return <><PageHeader title="Lịch cấp phát"
    actions={user.role === "ADMIN" ? <Button onClick={run} disabled={mutation.isPending}><Play className="size-4" />Cập nhật cấp phát</Button> : undefined} />
    <Card><CardHeader title="Workload đã được cấp tài nguyên" />
      {jobs.isLoading ? <TableSkeleton rows={4} /> : jobs.error ? <ErrorState message={jobs.error.message} /> : assigned.length === 0 ? <EmptyState icon={<Cpu />} title="Chưa có lịch cấp phát" description="Tạo workload để đăng ký tài nguyên." /> :
        <div className="overflow-x-auto"><table className="data-table"><thead><tr><th>Workload</th><th>Trạng thái</th><th>GPU</th><th>Bắt đầu</th><th>Kết thúc</th></tr></thead>
          <tbody>{assigned.slice(0, 20).map(job => <tr key={job.id}><td><Link className="font-semibold" href={"/workloads/" + job.id}>{job.name}</Link></td><td><Badge value={job.status} /></td><td>{job.assignment?.gpuUuids.length}</td><td>{formatDateTime(job.assignment?.startAt)}</td><td>{formatDateTime(job.assignment?.endAt)}</td></tr>)}</tbody>
        </table></div>}
    </Card>
  </>;
}
