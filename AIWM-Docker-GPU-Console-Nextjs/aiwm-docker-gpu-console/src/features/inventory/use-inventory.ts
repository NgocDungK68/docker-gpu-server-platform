"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";
import type { ContainerOrigin } from "@/lib/api/types";

export const useServers = () => useQuery({ queryKey: queryKeys.servers, queryFn: api.getServers });
export const useServer = (id: string) => useQuery({ queryKey: queryKeys.server(id), queryFn: () => api.getServer(id), enabled: Boolean(id) });
export const useGPUs = () => useQuery({ queryKey: queryKeys.gpus, queryFn: api.getGPUs });
export const useContainers = (origin?: ContainerOrigin) => useQuery({ queryKey: queryKeys.containers(origin), queryFn: () => api.getContainers(origin) });

export function useDrainServer(id: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (drained: boolean) => api.setServerDrained(id, drained),
    onSuccess: (server) => {
      client.setQueryData(queryKeys.server(id), server);
      void client.invalidateQueries({ queryKey: queryKeys.servers });
      void client.invalidateQueries({ queryKey: queryKeys.summary });
    },
  });
}
