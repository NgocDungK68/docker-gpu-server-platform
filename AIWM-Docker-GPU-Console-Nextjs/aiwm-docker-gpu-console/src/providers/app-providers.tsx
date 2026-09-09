"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { appConfig } from "@/config/app";
import { ToastProvider } from "@/components/ui/toast";

export function AppProviders({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            refetchInterval: appConfig.refreshIntervalMs,
            retry: 1,
            refetchOnWindowFocus: true,
            staleTime: 2_000,
          },
          mutations: { retry: 0 },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}
