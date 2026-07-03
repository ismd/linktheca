import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "@/shared/api/errors";

// Retry transient server errors, but never 501 Not Implemented: it signals a
// permanently disabled feature (e.g. radar_disabled), so retrying only delays
// the correct "disabled" state.
export function shouldRetryQuery(failureCount: number, err: unknown): boolean {
  if (!(err instanceof ApiError)) return false;
  if (err.status < 500 || err.status === 501) return false;
  return failureCount < 2;
}

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: shouldRetryQuery,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
    mutations: { retry: false },
  },
});
