"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export const useJobs = () => useQuery({ queryKey: queryKeys.jobs, queryFn: api.getJobs });
export const useJob = (id: string) => useQuery({ queryKey: queryKeys.job(id), queryFn: () => api.getJob(id), enabled: Boolean(id) });
export const useQueue = () => useQuery({ queryKey: queryKeys.queue, queryFn: api.getQueue });

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
