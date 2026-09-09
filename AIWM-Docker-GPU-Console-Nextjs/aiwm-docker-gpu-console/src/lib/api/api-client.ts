import { appConfig } from "@/config/app";
import { endpoints } from "@/lib/api/endpoints";
import type {
  AllocationOptions,
  JobPreview,
  APIEnvelope,
  APIErrorBody,
  ClusterSummary,
  ContainerInventoryItem,
  ContainerOrigin,
  CreateJobInput,
  GPUInventoryItem,
  HealthStatus,
  Job,
  SchedulerResult,
  Server,
} from "@/lib/api/types";

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code = "UNKNOWN",
    public readonly fields: Record<string, string> = {},
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${appConfig.apiProxyBasePath}${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });

  const payload = (await response.json().catch(() => ({}))) as
    | APIEnvelope<T>
    | APIErrorBody;
  if (!response.ok) {
    const error = "error" in payload ? payload.error : undefined;
    throw new ApiError(
      error?.message ?? `Request failed with status ${response.status}`,
      response.status,
      error?.code,
      error?.fields,
    );
  }
  return (payload as APIEnvelope<T>).data;
}

const liveApi = {
  health: async (): Promise<HealthStatus> => {
    const response = await fetch(appConfig.healthPath, { cache: "no-store" });
    return (await response.json()) as HealthStatus;
  },
  getSummary: () => request<ClusterSummary>(endpoints.summary),
  getServers: () => request<Server[]>(endpoints.servers),
  getServer: (id: string) => request<Server>(endpoints.server(id)),
  setServerDrained: (id: string, drained: boolean) =>
    request<Server>(endpoints.drainServer(id), {
      method: "POST",
      body: JSON.stringify({ drained }),
    }),
  getGPUs: () => request<GPUInventoryItem[]>(endpoints.gpus),
  getContainers: (origin?: ContainerOrigin) =>
    request<ContainerInventoryItem[]>(endpoints.containers(origin)),
  getAllocationOptions: () => request<AllocationOptions>(endpoints.jobOptions),
  previewJob: (input: CreateJobInput) => request<JobPreview>(endpoints.jobPreview, { method: "POST", body: JSON.stringify(input) }),
  getJobs: () => request<Job[]>(endpoints.jobs),
  getJob: (id: string) => request<Job>(endpoints.job(id)),
  createJob: (input: CreateJobInput) =>
    request<Job>(endpoints.jobs, { method: "POST", body: JSON.stringify(input) }),
  stopJob: (id: string) => request<Job>(endpoints.stopJob(id), { method: "POST" }),
  getQueue: () => request<Job[]>(endpoints.queue),
  runScheduler: () =>
    request<SchedulerResult>(endpoints.runScheduler, { method: "POST" }),
};

export type AIWMApi = typeof liveApi;
export const api: AIWMApi = liveApi;
