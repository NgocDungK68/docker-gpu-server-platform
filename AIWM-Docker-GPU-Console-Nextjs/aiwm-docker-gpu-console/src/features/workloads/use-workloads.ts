"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";

export function useJobs() { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.jobs, organizationFilter], queryFn: () => api.getJobs(organizationFilter), refetchInterval: 10000 }); }
export const useJob = (id: string) => useQuery({ queryKey: queryKeys.job(id), queryFn: () => api.getJob(id), enabled: Boolean(id), refetchInterval: 10000 });
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

// The shared clock updates time labels; warning thresholds remain backend-owned.
export function useWorkloadNow() {
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    const initial = setTimeout(() => setNow(Date.now()), 0);
    const timer = setInterval(() => setNow(Date.now()), 10000);
    return () => { clearTimeout(initial); clearInterval(timer); };
  }, []);
  return now;
}

export function useContinueJob(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { neededAt: string; ttlSeconds: number }) => api.continueJob(id, input),
    onSuccess: (job) => {
      client.setQueryData(queryKeys.job(job.id), job);
      void client.invalidateQueries({ queryKey: queryKeys.jobs });
      void client.invalidateQueries({ queryKey: queryKeys.queue });
    },
  });
}
