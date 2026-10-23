"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/api-client";
import { queryKeys } from "@/lib/api/query-keys";
import { appConfig } from "@/config/app";
import { cn } from "@/lib/utils/cn";

export function ConnectionStatus({ compact = false }: { compact?: boolean }) {
  const query = useQuery({ queryKey: queryKeys.health, queryFn: api.health, refetchInterval: appConfig.refreshIntervalMs });
  const connected = query.data?.status === "ok";
  return <div className={cn("inline-flex items-center gap-2 rounded-full border px-2.5 py-1.5 text-xs font-semibold", connected ? "border-emerald-200 bg-emerald-50 text-emerald-700" : "border-red-200 bg-red-50 text-red-700")} title="Kết nối hệ thống"><span className={cn("size-2 rounded-full", connected ? "bg-emerald-500" : "bg-red-500")} />{!compact && (connected ? "Đã kết nối" : "Mất kết nối")}</div>;
}
