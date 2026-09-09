export const queryKeys = {
  health: ["health"] as const,
  summary: ["summary"] as const,
  servers: ["servers"] as const,
  server: (id: string) => ["servers", id] as const,
  gpus: ["gpus"] as const,
  containers: (origin?: string) => ["containers", origin ?? "ALL"] as const,
  jobs: ["jobs"] as const,
  job: (id: string) => ["jobs", id] as const,
  queue: ["queue"] as const,
};
