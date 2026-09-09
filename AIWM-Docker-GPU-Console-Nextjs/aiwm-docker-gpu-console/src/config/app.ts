export const appConfig = {
  name: "AIWM GPU Console",
  organization: "Viettel AI Platform",
  apiProxyBasePath: "/api/aiwm",
  healthPath: "/api/aiwm-health",
  dataMode: "live",
  refreshIntervalMs: refreshInterval(),
} as const;

export type DataMode = typeof appConfig.dataMode;

function refreshInterval() {
 const value = Number(process.env.NEXT_PUBLIC_REFRESH_INTERVAL_MS ?? 5000);
 if (!Number.isFinite(value) || value < 500) throw new Error("NEXT_PUBLIC_REFRESH_INTERVAL_MS must be at least 500");
 return value;
}
