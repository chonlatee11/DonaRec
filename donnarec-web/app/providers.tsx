"use client";

import { useState } from "react";
import {
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";

/**
 * Providers — client-side context wrapper mounted in the root layout.
 *
 * Instantiates a single QueryClient per browser session (via useState so the
 * instance is stable across re-renders and never recreated on the client),
 * then wraps children in QueryClientProvider so client components under
 * AppShell can use TanStack Query hooks (useQuery/useMutation).
 *
 * D-R1: TanStack Query drives client cache / refetch / pagination against the
 * BFF Route Handlers (app/api/bff/**). The Keycloak access token is obtained
 * server-side inside those handlers and never reaches this client boundary.
 */
export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Back-office data changes on human review actions — avoid noisy
            // background refetch while keeping cache useful across navigation.
            staleTime: 30_000,
            refetchOnWindowFocus: false,
            retry: 1,
          },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}
