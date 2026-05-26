import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  useTopicsQuery,
  useTopicQuery,
  useMatchesQuery,
  useMatchQuery,
  useRadarStatusQuery,
} from "./use-radar";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

const rawTopic = (id: number) => ({
  id, user_id: 1, name: `T${id}`, description: "D",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
});

const rawMatch = (id: number) => ({
  id, topic_id: 1, topic_name: "T1", similarity: 0.7, state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 200, feed_id: 5, feed_title: "F",
    url: "https://x.example/a", title: "Title", summary: null,
    published_at: null, discovered_at: "2026-05-18T09:00:00Z",
  },
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("useTopicsQuery", () => {
  it("loads topics array", async () => {
    server.use(
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(1), rawTopic(2)] })),
    );
    const { result } = renderHook(() => useTopicsQuery(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
  });
});

describe("useTopicQuery", () => {
  it("loads single topic", async () => {
    server.use(
      http.get("/api/radar/topics/7", () => HttpResponse.json(rawTopic(7))),
    );
    const { result } = renderHook(() => useTopicQuery(7), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe(7);
  });
});

describe("useMatchesQuery", () => {
  it("first page with topicId filter", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({
          items: [rawMatch(1), rawMatch(2)],
          total: 2,
        });
      }),
    );
    const { result } = renderHook(
      () => useMatchesQuery({ topicId: 1 }),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(capturedUrl).toContain("topic_id=1");
    expect(result.current.items).toHaveLength(2);
    expect(result.current.hasMore).toBe(false);
  });

  it("computes hasMore when total > loaded", async () => {
    server.use(
      http.get("/api/radar/matches", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawMatch(i + 1)),
          total: 55,
        })),
    );
    const { result } = renderHook(() => useMatchesQuery({}), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasMore).toBe(true);
  });
});

describe("useMatchQuery", () => {
  it("loads single match", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch(42))),
    );
    const { result } = renderHook(() => useMatchQuery(42), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe(42);
  });
});

describe("useRadarStatusQuery", () => {
  it("loads last_sweep_at", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: "2026-05-18T10:00:00Z" })),
    );
    const { result } = renderHook(() => useRadarStatusQuery(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.lastSweepAt).toBeInstanceOf(Date);
  });
});
