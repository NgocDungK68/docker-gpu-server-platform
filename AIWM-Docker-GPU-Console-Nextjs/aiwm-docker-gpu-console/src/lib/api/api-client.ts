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

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
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
    if (response.status === 401 && typeof window !== "undefined" && window.location.pathname !== "/login") window.location.replace("/login");
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

const scoped = (path: string, organizationId?: string) => organizationId ? `${path}${path.includes("?") ? "&" : "?"}organizationId=${encodeURIComponent(organizationId)}` : path;

const liveApi = {
  health: async (): Promise<HealthStatus> => {
    const response = await fetch(appConfig.healthPath, { cache: "no-store" });
    return (await response.json()) as HealthStatus;
  },
  getSummary: (organizationId?: string) => request<ClusterSummary>(scoped(endpoints.summary, organizationId)),
  getServers: (organizationId?: string) => request<Server[]>(scoped(endpoints.servers, organizationId)),
  getServer: (id: string) => request<Server>(endpoints.server(id)),
  setServerDrained: (id: string, drained: boolean) =>
    request<Server>(endpoints.drainServer(id), {
      method: "POST",
      body: JSON.stringify({ drained }),
    }),
  getGPUs: (organizationId?: string) => request<GPUInventoryItem[]>(scoped(endpoints.gpus, organizationId)),
  getContainers: (origin?: ContainerOrigin, organizationId?: string) =>
    request<ContainerInventoryItem[]>(scoped(endpoints.containers(origin), organizationId)),
  getAllocationOptions: () => request<AllocationOptions>(endpoints.jobOptions),
  previewJob: (input: CreateJobInput, signal?: AbortSignal) => request<JobPreview>(endpoints.jobPreview, { method: "POST", body: JSON.stringify(input), signal }),
  getJobs: (organizationId?: string) => request<Job[]>(scoped(endpoints.jobs, organizationId)),
  getJob: (id: string) => request<Job>(endpoints.job(id)),
  createJob: (input: CreateJobInput) =>
    request<Job>(endpoints.jobs, { method: "POST", body: JSON.stringify(input) }),
  stopJob: (id: string) => request<Job>(endpoints.stopJob(id), { method: "POST" }),
  getQueue: (organizationId?: string) => request<Job[]>(scoped(endpoints.queue, organizationId)),
  runScheduler: () =>
    request<SchedulerResult>(endpoints.runScheduler, { method: "POST" }),
};

export type AIWMApi = typeof liveApi;
export const api: AIWMApi = liveApi;
