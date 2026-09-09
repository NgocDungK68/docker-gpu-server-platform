export const endpoints = {
  summary: "/system/summary",
  servers: "/servers",
  server: (serverId: string) => `/servers/${serverId}`,
  drainServer: (serverId: string) => `/servers/${serverId}/drain`,
  gpus: "/gpus",
  containers: (origin?: string) => `/containers${origin ? `?origin=${origin}` : ""}`,
  jobs: "/jobs",
  job: (jobId: string) => `/jobs/${jobId}`,
  stopJob: (jobId: string) => `/jobs/${jobId}/stop`,
  queue: "/queue",
  runScheduler: "/scheduler/run-once",
} as const;
