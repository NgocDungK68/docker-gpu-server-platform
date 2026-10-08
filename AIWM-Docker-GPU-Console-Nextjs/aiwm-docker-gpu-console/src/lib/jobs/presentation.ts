import type { Job, Server } from "../api/types";

export type WorkloadTone = "success" | "warning" | "danger" | "info" | "neutral";
export interface WorkloadDisplay { label: string; tone: WorkloadTone; key: string; }

function timestamp(value?: string): number | null {
  if (!value || value.startsWith("0001-")) return null;
  const n = Date.parse(value);
  return Number.isFinite(n) ? n : null;
}

export function allocationWindow(job: Job) {
  const start = timestamp(job.assignment?.startAt) ?? timestamp(job.requestedStartAt);
  const end = timestamp(job.assignment?.endAt) ?? timestamp(job.requestedEndAt);
  return start !== null && end !== null && end > start ? { start, end } : null;
}

export function canContinue(job: Job): boolean {
  const eligible = job.workloadType === "TRAINING" && job.status === "STOPPED" &&
    job.terminationReason === "TIME_LIMIT" && job.training?.checkpointStatus === "AVAILABLE";
  return eligible && (job.resumable ?? Boolean(job.training?.latestCheckpointURI));
}

export function workloadWarning(job: Job, now: number | null): boolean {
  const warning = timestamp(job.training?.checkpointWarningAt);
  return job.status === "RUNNING" && (job.training?.checkpointStatus === "REQUESTED" ||
    (now !== null && warning !== null && now >= warning));
}

export function workloadDisplay(job: Job, now: number | null): WorkloadDisplay {
  switch (job.status) {
    case "QUEUED": return { key: "waiting", label: "Đang chờ", tone: "neutral" };
    case "ASSIGNED": return { key: "planned", label: "Đã lên lịch", tone: "info" };
    case "STARTING": return { key: "starting", label: "Đang khởi động", tone: "info" };
    case "FAILED": return { key: "failed", label: "Thất bại", tone: "danger" };
    case "CANCELLED": return { key: "cancelled", label: "Đã hủy", tone: "neutral" };
    case "STOPPING": return { key: "stopping", label: job.terminationReason === "TIME_LIMIT" ? "Đang thu hồi" : "Đang dừng", tone: "warning" };
    case "STOPPED":
      if (job.terminationReason === "TIME_LIMIT") return canContinue(job)
        ? { key: "resumable", label: "Có thể tiếp tục", tone: "info" }
        : { key: "expired", label: "Hết thời gian", tone: "danger" };
      return { key: "stopped", label: "Đã dừng", tone: "neutral" };
    case "SUCCEEDED":
      if (job.workloadType === "TRAINING" && job.training?.artifactStatus !== "READY") return job.training?.artifactStatus === "FAILED"
        ? { key: "output-failed", label: "Lỗi lưu model", tone: "danger" }
        : { key: "saving-model", label: "Đang lưu model", tone: "info" };
      return { key: "completed", label: "Hoàn thành", tone: "success" };
    case "RUNNING": {
      if (job.training?.checkpointStatus === "SAVING") return { key: "saving-checkpoint", label: "Đang lưu checkpoint", tone: "info" };
      if (workloadWarning(job, now))
        return { key: "warning", label: "Cảnh báo", tone: "warning" };
      return { key: "running", label: "Đang chạy", tone: "success" };
    }
    default: return { key: "unknown", label: "Chưa có trạng thái", tone: "neutral" };
  }
}

export function allocationProgress(job: Job, now: number | null) {
  if (job.terminationReason === "TIME_LIMIT") return { percent: 100, tone: "danger" as const };
  const window = allocationWindow(job);
  if (!window || now === null) return null;
  const ratio = Math.min(1, Math.max(0, (now - window.start) / (window.end - window.start)));
  return { percent: Math.floor(ratio * 100), tone: ratio >= .9 ? "danger" as const : ratio >= .7 ? "warning" as const : "success" as const };
}

function timeAmount(ms: number): string {
  const seconds = Math.max(1, Math.ceil(ms / 1000));
  if (seconds >= 86400) return Math.ceil(seconds / 86400) + " ngày";
  if (seconds >= 3600) return Math.ceil(seconds / 3600) + " giờ";
  if (seconds >= 60) return Math.ceil(seconds / 60) + " phút";
  return seconds + " giây";
}

export function allocationTimeLabel(job: Job, now: number | null): string {
  if (job.status === "SUCCEEDED") return "Đã hoàn thành";
  if (job.status === "CANCELLED") return "Đã hủy";
  if (job.status === "FAILED") return "Đã kết thúc";
  if (job.status === "STOPPED" && job.terminationReason !== "TIME_LIMIT") return "Đã dừng";
  const window = allocationWindow(job);
  if (!window || now === null) return "—";
  if (now >= window.end) return now === window.end ? "Vừa hết hạn" : "Hết hạn " + timeAmount(now - window.end) + " trước";
  if (job.terminationReason === "TIME_LIMIT") return "Đã hết hạn";
  if (now < window.start) return "Bắt đầu sau " + timeAmount(window.start - now);
  return "Còn " + timeAmount(window.end - now);
}

export function workloadGPU(job: Job, servers: Server[] = []): string {
  const assignment = job.assignment;
  if (assignment?.gpuUuids?.length) {
    const server = servers.find(s => s.id === assignment.serverId && s.organizationId === job.organizationId);
    const models = new Map<string, number>();
    let unknown = 0;
    for (const uuid of assignment.gpuUuids) {
      const model = server?.gpus?.find(g => g.uuid === uuid)?.model;
      if (model) models.set(model, (models.get(model) ?? 0) + 1); else unknown++;
    }
    const parts = [...models].sort(([a], [b]) => a.localeCompare(b)).map(([model, count]) => count + " × " + model);
    if (unknown) parts.push(unknown + " GPU");
    return parts.join(" · ");
  }
  const count = job.resources.gpuCount + " GPU";
  return job.resources.performanceProfile === "HIGH_PERFORMANCE" ? count + " · Hiệu năng cao" : count + " · Auto";
}

export function workloadTypeLabel(job: Job): string {
  return job.workloadType === "TRAINING" ? "Training" : job.workloadType === "INFERENCE" ? "Inference" : "—";
}

export function checkpointLabel(job: Job): string {
  switch (job.training?.checkpointStatus) {
    case "REQUESTED": return "Đã yêu cầu lưu";
    case "SAVING": return "Đang lưu";
    case "AVAILABLE": return "Đã lưu";
    case "FAILED": return "Không lưu được";
    default: return "Chưa có checkpoint";
  }
}

export function terminationLabel(job: Job): string | null {
  const reasons = { COMPLETED: "Hoàn thành", TIME_LIMIT: "Hết thời gian cấp phát", USER_CANCELLED: "Theo yêu cầu của người dùng", EXECUTION_ERROR: "Workload thực thi không thành công", SYSTEM_ERROR: "Không thể hoàn tất do lỗi hệ thống" };
  return job.terminationReason ? reasons[job.terminationReason] : null;
}
