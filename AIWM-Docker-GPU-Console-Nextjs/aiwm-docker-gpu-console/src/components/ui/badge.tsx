import { cn } from "@/lib/utils/cn";

const toneMap: Record<string, string> = {
  ONLINE: "badge-success", RUNNING: "badge-success", SUCCEEDED: "badge-success", FREE: "badge-success",
  QUEUED: "badge-warning", RESERVED: "badge-warning", STARTING: "badge-info", ASSIGNED: "badge-info", STOPPING: "badge-info",
  CANCELLED: "badge-neutral", OFFLINE: "badge-neutral", STOPPED: "badge-neutral", LEGACY: "badge-violet", MANAGED: "badge-info",
  DRAINING: "badge-warning", ALLOCATED: "badge-info", OCCUPIED_LEGACY: "badge-violet", OCCUPIED_UNKNOWN: "badge-danger", UNKNOWN: "badge-danger", UNHEALTHY: "badge-danger", FAILED: "badge-danger",
};

export function Badge({ value, label, className }: { value: string; label?: string; className?: string }) {
  return <span className={cn("badge", toneMap[value] ?? "badge-neutral", className)}>{label ?? (value === "LEGACY" ? "External" : value.replaceAll("_", " "))}</span>;
}
