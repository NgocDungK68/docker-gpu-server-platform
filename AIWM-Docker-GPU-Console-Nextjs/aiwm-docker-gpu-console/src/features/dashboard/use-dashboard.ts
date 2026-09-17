"use client";

import { useQueries } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export function useDashboard() {
  const { organizationFilter } = useSession();
  const [summary, servers, gpus, jobs] = useQueries({
    queries: [
      { queryKey: [...queryKeys.summary, organizationFilter], queryFn: () => api.getSummary(organizationFilter) },
      { queryKey: [...queryKeys.servers, organizationFilter], queryFn: () => api.getServers(organizationFilter) },
      { queryKey: [...queryKeys.gpus, organizationFilter], queryFn: () => api.getGPUs(organizationFilter) },
      { queryKey: [...queryKeys.jobs, organizationFilter], queryFn: () => api.getJobs(organizationFilter) },
    ],
  });
  return {
    summary: summary.data,
    servers: servers.data ?? [],
    gpus: gpus.data ?? [],
    jobs: jobs.data ?? [],
    isLoading: summary.isLoading || servers.isLoading || gpus.isLoading || jobs.isLoading,
    error: summary.error ?? servers.error ?? gpus.error ?? jobs.error,
    refetch: () => Promise.all([summary.refetch(), servers.refetch(), gpus.refetch(), jobs.refetch()]),
  };
}
