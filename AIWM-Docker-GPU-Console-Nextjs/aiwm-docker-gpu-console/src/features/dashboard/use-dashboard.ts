"use client";

import { useQueries } from "@tanstack/react-query";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export function useDashboard() {
  const [summary, servers, gpus, jobs] = useQueries({
    queries: [
      { queryKey: queryKeys.summary, queryFn: api.getSummary },
      { queryKey: queryKeys.servers, queryFn: api.getServers },
      { queryKey: queryKeys.gpus, queryFn: api.getGPUs },
      { queryKey: queryKeys.jobs, queryFn: api.getJobs },
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
