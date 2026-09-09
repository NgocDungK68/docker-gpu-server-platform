export type ServerStatus = "ONLINE" | "OFFLINE" | "DRAINING";
export type GPUState =
  | "FREE"
  | "RESERVED"
  | "ALLOCATED"
  | "OCCUPIED_LEGACY"
  | "OCCUPIED_UNKNOWN"
  | "UNHEALTHY";
export type ContainerOrigin = "MANAGED" | "LEGACY" | "UNKNOWN";
export type JobStatus =
  | "QUEUED"
  | "ASSIGNED"
  | "STARTING"
  | "RUNNING"
  | "STOPPING"
  | "STOPPED"
  | "CANCELLED"
  | "SUCCEEDED"
  | "FAILED";
export type SchedulingStrategy = "first-fit" | "best-fit" | "bin-pack" | "fragmentation-aware";

export interface ClusterSummary {
  serversTotal: number;
  serversOnline: number;
  gpusTotal: number;
  gpusFree: number;
  gpusOccupiedLegacy: number;
  gpusOccupiedUnknown: number;
  jobsQueued: number;
  jobsRunning: number;
  gpusOccupied: number;
  recentEvents: JobEvent[];
}

export interface GPU {
  uuid: string;
  index: number;
  model: string;
  memoryTotalMiB: number;
  memoryUsedMiB: number;
  utilizationPct: number;
  temperatureC?: number;
  healthy: boolean;
  state: GPUState;
  assignedJobId?: string;
  observedConsumers?: string[];
  stateReason?: string;
}

export interface Container {
  id: string;
  name: string;
  image: string;
  state: string;
  origin: ContainerOrigin;
  jobId?: string;
  gpuUuids?: string[];
  labels?: Record<string, string>;
  startedAt?: string;
  finishedAt?: string;
  exitCode?: number;
}

export interface Server {
  schedulable: boolean;
  schedulingReason: string;
  host: { hostname: string; os: string; architecture: string; cpuCount: number; memoryTotalMiB: number };
  id: string;
  machineId: string;
  name: string;
  address?: string;
  agentVersion?: string;
  labels?: Record<string, string>;
  status: ServerStatus;
  drained: boolean;
  lastHeartbeatAt: string;
  lastInventoryAt?: string;
  inventoryVersion: number;
  inventoryReceivedAt?: string;
  dockerVersion?: string;
  gpus: GPU[];
  containers: Container[];
}

export interface GPUInventoryItem {
  serverId: string;
  serverName: string;
  serverStatus: ServerStatus;
  gpu: GPU;
}

export interface ContainerInventoryItem {
  serverId: string;
  serverName: string;
  container: Container;
}

export interface ResourceRequest {
  gpuCount: number;
  gpuModel?: string;
  minVramMiB?: number;
  cpuMilli?: number;
  memoryMiB?: number;
  allowSharedGpu: boolean;
}

export interface Assignment {
  serverId: string;
  gpuUuids: string[];
  commandId: string;
  assignedAt: string;
  reservationState: "RESERVED" | "ALLOCATED" | "RELEASED";
  releasedAt?: string;
  strategy: SchedulingStrategy;
  score: number;
  reason: string;
}

export interface Job {
  id: string;
  name: string;
  image: string;
  backend: "DOCKER";
  command?: string[];
  environment?: Record<string, string>;
  resources: ResourceRequest;
  priority: number;
  serverSelector?: Record<string, string>;
  strategy: SchedulingStrategy;
  status: JobStatus;
  statusReason?: string;
  assignment?: Assignment;
  createdAt: string;
  updatedAt: string;
  lastObservedAt?: string;
  containerId?: string;
  events: JobEvent[];
}

export interface CreateJobInput {
  name: string;
  image: string;
  backend: "DOCKER";
  command?: string[];
  environment?: Record<string, string>;
  resources: ResourceRequest;
  priority: number;
  serverSelector?: Record<string, string>;
  strategy: SchedulingStrategy;
}

export interface APIErrorBody {
  error?: { code: string; message: string };
}

export interface APIEnvelope<T> {
  data: T;
  meta?: unknown;
}

export interface SchedulerResult {
  assignedJobs: number;
}

export interface HealthStatus {
  status: "ok" | "unreachable";
  backendUrl?: string;
  detail?: string;
}

export interface JobEvent { jobId: string; status: JobStatus; reason: string; at: string; }
