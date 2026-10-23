import { Badge } from "@/components/ui/badge";
import type { OperatingMode } from "@/lib/api/types";

const modes: Record<OperatingMode, { label: string; tone: string }> = {
  FULL: { label: "Đầy đủ chức năng", tone: "FREE" },
  GPU_OBSERVE_ONLY: { label: "Chỉ giám sát GPU", tone: "ASSIGNED" },
  DOCKER_OBSERVE_ONLY: { label: "Chỉ giám sát container", tone: "ASSIGNED" },
  DEGRADED: { label: "Giới hạn chức năng", tone: "RESERVED" },
};

export function ServerMode({ mode }: { mode?: OperatingMode }) {
  const presentation = mode ? modes[mode] : undefined;
  return <Badge value={presentation?.tone ?? "UNKNOWN"} label={presentation?.label ?? "Chưa xác định chức năng"} />;
}
