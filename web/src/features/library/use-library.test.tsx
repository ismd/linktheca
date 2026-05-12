import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { useLibraryQuery, useLibraryItemDetailQuery } from "./use-library";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const rawItem = (id: number) => ({
  id,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: `2026-05-${String(id).padStart(2, "0")}T12:00:00Z`,
  read_at: null,
  url: `https://example.com/${id}`,
  title: `Title ${id}`,
  excerpt: "ex",
  reading_time_seconds: 120,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("useLibraryQuery", () => {
  it("loads first page with given filters", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({
          items: [rawItem(1), rawItem(2)],
          total: 2,
        });
      }),
    );

    const { result } = renderHook(
      () => useLibraryQuery({ state: "unread" }),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(capturedUrl).toContain("state=unread");
    expect(capturedUrl).toContain("offset=0");
    expect(result.current.items).toHaveLength(2);
    expect(result.current.total).toBe(2);
    expect(result.current.hasMore).toBe(false);
  });

  it("computes hasMore=true when total > items loaded", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawItem(i + 1)),
          total: 55,
        }),
      ),
    );

    const { result } = renderHook(() => useLibraryQuery({}), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasMore).toBe(true);
    expect(result.current.items).toHaveLength(20);
  });
});

describe("useLibraryItemDetailQuery", () => {
  it("loads item detail by id", async () => {
    server.use(
      http.get("/api/library/3/content", () =>
        HttpResponse.json({
          ...rawItem(3),
          content: {
            id: 99,
            url: "https://example.com/3",
            canonical_url: null,
            title: "T",
            byline: null,
            excerpt: null,
            text: "body",
            html: "<p>body</p>",
            lang: "en",
            reading_time_seconds: 120,
            fetched_at: "2026-05-10T12:00:00Z",
            fetch_error: null,
          },
        }),
      ),
    );

    const { result } = renderHook(() => useLibraryItemDetailQuery(3), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.content.html).toBe("<p>body</p>");
  });
});
