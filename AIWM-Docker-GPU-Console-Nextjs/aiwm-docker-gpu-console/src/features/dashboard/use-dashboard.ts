"use client";

import { useQuery } from "@tanstack/react-query";
import { useSession } from "@/features/identity/session";
import { api } from "@/lib/api/api-client";
import { identityApi } from "@/lib/api/identity";
import { queryKeys } from "@/lib/api/query-keys";
import { appConfig } from "@/config/app";

export function useDashboard() {
  const { user, organizationFilter } = useSession();
  const admin = user.role === "ADMIN";
  const servers = useQuery({
    queryKey: [...queryKeys.servers, organizationFilter],
    queryFn: () => api.getServers(organizationFilter),
    refetchInterval: appConfig.refreshIntervalMs,
  });
  const organizations = useQuery({ queryKey: ["organizations"], queryFn: identityApi.organizations, enabled: admin });
  return {
    admin,
    servers: servers.data ?? [],
    organizations: (organizations.data ?? []).filter(o => !organizationFilter || o.id === organizationFilter),
    isLoading: servers.isPending || (admin && organizations.isPending),
    error: servers.error ?? (admin ? organizations.error : null),
    refetch: () => Promise.all([servers.refetch(), ...(admin ? [organizations.refetch()] : [])]),
  };
}
