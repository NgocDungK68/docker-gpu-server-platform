"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";
import type { ContainerOrigin } from "@/lib/api/types";

export function useServers() { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.servers, organizationFilter], queryFn: () => api.getServers(organizationFilter) }); }
export const useServer = (id: string) => useQuery({ queryKey: queryKeys.server(id), queryFn: () => api.getServer(id), enabled: Boolean(id) });
export function useGPUs() { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.gpus, organizationFilter], queryFn: () => api.getGPUs(organizationFilter) }); }
export function useContainers(origin?: ContainerOrigin) { const { organizationFilter } = useSession(); return useQuery({ queryKey: [...queryKeys.containers(origin), organizationFilter], queryFn: () => api.getContainers(origin, organizationFilter) }); }

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
