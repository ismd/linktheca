import { describe, it, expect } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useGlobalFeedsQuery } from "./use-admin-feeds";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    qc,
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  };
}

describe("useGlobalFeedsQuery", () => {
  it("reads the admin catalog endpoint", async () => {
    server.use(
      http.get("/api/admin/radar/feeds", () =>
        HttpResponse.json({
          items: [
            {
              id: 1, url: "https://theverge.com/rss", kind: "rss", title: "The Verge",
              fetch_interval_seconds: 3600, is_active: true,
              last_fetched_at: null, last_error: null,
              created_at: "2026-08-01T10:00:00Z",
              subscribed: false, finding_count: 214, is_own: false,
            },
          ],
          total: 1,
        }),
      ),
    );

    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useGlobalFeedsQuery(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0]!.title).toBe("The Verge");
  });
});
