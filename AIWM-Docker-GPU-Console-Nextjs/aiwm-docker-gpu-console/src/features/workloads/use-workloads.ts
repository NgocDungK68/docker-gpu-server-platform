"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export function useJobs() { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.jobs, organizationFilter], queryFn: () => api.getJobs(organizationFilter) }); }
export const useJob = (id: string) => useQuery({ queryKey: queryKeys.job(id), queryFn: () => api.getJob(id), enabled: Boolean(id) });
export function useQueue() { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.queue, organizationFilter], queryFn: () => api.getQueue(organizationFilter) }); }

export function useCreateJob() {
  const client = useQueryClient();
  return useMutation({ mutationFn: api.createJob, onSuccess: () => { void client.invalidateQueries({ queryKey: queryKeys.jobs }); void client.invalidateQueries({ queryKey: queryKeys.queue }); void client.invalidateQueries({ queryKey: queryKeys.summary }); } });
}

export function useStopJob(id: string) {
  const client = useQueryClient();
  return useMutation({ mutationFn: () => api.stopJob(id), onSuccess: (job) => { client.setQueryData(queryKeys.job(id), job); void client.invalidateQueries({ queryKey: queryKeys.jobs }); void client.invalidateQueries({ queryKey: queryKeys.summary }); } });
}

export function useRunScheduler() {
  const client = useQueryClient();
  return useMutation({ mutationFn: api.runScheduler, onSuccess: () => { void client.invalidateQueries({ queryKey: queryKeys.jobs }); void client.invalidateQueries({ queryKey: queryKeys.queue }); void client.invalidateQueries({ queryKey: queryKeys.summary }); void client.invalidateQueries({ queryKey: queryKeys.gpus }); void client.invalidateQueries({ queryKey: queryKeys.servers }); } });
}
