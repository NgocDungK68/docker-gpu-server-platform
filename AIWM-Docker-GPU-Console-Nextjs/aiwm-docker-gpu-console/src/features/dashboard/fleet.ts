import type { Server } from "../../lib/api/types";

export function fleetMetrics(servers: Server[]) {
  const metrics = { servers: servers.length, online: 0, total: 0, free: 0, reserved: 0, allocated: 0, legacy: 0, unknown: 0, unhealthy: 0, telemetryCount: 0, utilizationSum: 0, averageUtilization: null as number | null };
  for (const server of servers) {
    if (server.status !== "OFFLINE") metrics.online++;
    const received = Date.parse(server.inventoryReceivedAt ?? "");
    for (const gpu of server.gpus) {
      metrics.total++;
      if (server.schedulable && gpu.healthy && gpu.state === "FREE" && !gpu.assignedJobId) metrics.free++;
      if (gpu.state === "RESERVED") metrics.reserved++;
      if (gpu.state === "ALLOCATED") metrics.allocated++;
      if (gpu.state === "OCCUPIED_LEGACY") metrics.legacy++;
      if (gpu.state === "OCCUPIED_UNKNOWN") metrics.unknown++;
      if (gpu.state === "UNHEALTHY" || !gpu.healthy) metrics.unhealthy++;
      // Latest reported sample only: exclude offline hosts, missing inventory and invalid GPU readings.
      if (server.status !== "OFFLINE" && Number.isFinite(received) && received > 0 && gpu.healthy && gpu.state !== "UNHEALTHY"
        && typeof gpu.utilizationPct === "number" && Number.isFinite(gpu.utilizationPct) && gpu.utilizationPct >= 0 && gpu.utilizationPct <= 100) {
        metrics.telemetryCount++;
        metrics.utilizationSum += gpu.utilizationPct;
      }
    }
  }
  if (metrics.telemetryCount) metrics.averageUtilization = metrics.utilizationSum / metrics.telemetryCount;
  return metrics;
}
