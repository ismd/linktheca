import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { ApiError } from "@/shared/api/errors";
import { radarKeys, useFeedsQuery } from "./use-radar";
import type { FeedListItem } from "./types";
import {
  useCreateTopic,
  useUpdateTopic,
  useDeleteTopic,
  useMarkMatchSeen,
  useToggleSubscription,
} from "./use-mutations";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper };
}

const rawTopic = (id: number, overrides: Record<string, unknown> = {}) => ({
  id, user_id: 1, name: `T${id}`, description: "D",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("useCreateTopic", () => {
  it("on success invalidates topics list", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(rawTopic(99), { status: 201 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topics, []);

    const { result } = renderHook(() => useCreateTopic(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ name: "X", description: "Y enough chars" });
    });
    await waitFor(() =>
      expect(qc.getQueryState(radarKeys.topics)?.isInvalidated).toBe(true),
    );
  });

  it("503 embedder_unavailable surfaces as ApiError with code", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(
          { error: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useCreateTopic(), { wrapper });
    let caught: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({ name: "X", description: "Y enough chars" });
      } catch (e) {
        caught = e;
      }
    });
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).code).toBe("embedder_unavailable");
    expect((caught as ApiError).status).toBe(503);
  });
});

describe("useUpdateTopic (optimistic isActive toggle)", () => {
  it("optimistically updates topics list cache", async () => {
    let resolve!: (v: Response) => void;
    const slow = new Promise<Response>((r) => (resolve = r));
    server.use(http.patch("/api/radar/topics/1", () => slow));

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topics, [
      mapTopic(rawTopic(1, { is_active: true })),
      mapTopic(rawTopic(2)),
    ]);

    const { result } = renderHook(() => useUpdateTopic(), { wrapper });
    act(() => {
      result.current.mutate({ id: 1, input: { isActive: false } });
    });

    await waitFor(() => {
      const list = qc.getQueryData<ReturnType<typeof mapTopic>[]>(radarKeys.topics);
      expect(list![0]!.isActive).toBe(false);
    });

    resolve(HttpResponse.json(rawTopic(1, { is_active: false })));
  });

  it("resolves when backend returns a bare topic without stats", async () => {
    // Real backend PATCH /radar/topics/{id} returns a bare Topic (no stats),
    // matching the Library convention. The mutation must not error on it.
    const bare = rawTopic(1, { is_active: false }) as Record<string, unknown>;
    delete bare.stats;
    server.use(http.patch("/api/radar/topics/1", () => HttpResponse.json(bare)));

    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useUpdateTopic(), { wrapper });

    let caught: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({ id: 1, input: { isActive: false } });
      } catch (e) {
        caught = e;
      }
    });
    expect(caught).toBeUndefined();
  });
});

describe("useDeleteTopic", () => {
  it("removes topic cache entries on success", async () => {
    server.use(
      http.delete("/api/radar/topics/5", () =>
        new HttpResponse(null, { status: 204 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topic(5), mapTopic(rawTopic(5)));

    const { result } = renderHook(() => useDeleteTopic(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(5);
    });

    expect(qc.getQueryData(radarKeys.topic(5))).toBeUndefined();
  });
});

describe("useMarkMatchSeen", () => {
  it("on success invalidates matches + topics + match queries", async () => {
    server.use(
      http.patch("/api/radar/matches/42", () =>
        new HttpResponse(null, { status: 204 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.match(42), {});
    qc.setQueryData(radarKeys.matches({}), { pages: [{ items: [], total: 0 }], pageParams: [0] });
    qc.setQueryData(radarKeys.topics, []);

    const { result } = renderHook(() => useMarkMatchSeen(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(42);
    });

    await waitFor(() => {
      expect(qc.getQueryState(radarKeys.match(42))?.isInvalidated).toBe(true);
      expect(qc.getQueryState(radarKeys.topics)?.isInvalidated).toBe(true);
    });
  });
});

// helper duplicated locally to avoid coupling tests to mapper internals
function mapTopic(raw: ReturnType<typeof rawTopic>) {
  return {
    id: raw.id,
    userId: raw.user_id,
    name: raw.name,
    description: raw.description,
    matchThreshold: raw.match_threshold,
    isActive: raw.is_active,
    hasEmbedding: raw.has_embedding,
    createdAt: new Date(raw.created_at),
    updatedAt: new Date(raw.updated_at),
    stats: {
      newCount: raw.stats.new_count,
      totalCount: raw.stats.total_count,
      sourceCount: raw.stats.source_count,
      lastMatchAt: raw.stats.last_match_at ? new Date(raw.stats.last_match_at) : null,
    },
  };
}

const rawFeed = (id: number, subscribed: boolean) => ({
  id, url: `https://f${id}.example/rss`, kind: "rss", title: `Feed ${id}`,
  fetch_interval_seconds: 3600, is_active: true,
  last_fetched_at: null, last_error: null, created_at: "2026-08-01T10:00:00Z",
  subscribed, finding_count: 0,
});

describe("useToggleSubscription", () => {
  it("rolls the subscription toggle back when the request fails", async () => {
    server.use(
      http.get("/api/radar/feeds", () =>
        HttpResponse.json({ items: [rawFeed(3, false)], total: 1 })),
      http.post("/api/radar/subscriptions", () =>
        HttpResponse.json({ error: "internal" }, { status: 500 })),
    );

    const { qc, wrapper } = makeWrapper();
    const { result } = renderHook(
      () => ({ feeds: useFeedsQuery(), toggle: useToggleSubscription() }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.feeds.isSuccess).toBe(true));

    await act(async () => {
      await result.current.toggle
        .mutateAsync({ feedId: 3, subscribed: true })
        .catch(() => undefined);
    });

    const cached = qc.getQueryData<FeedListItem[]>(radarKeys.feeds);
    expect(cached?.[0]!.subscribed).toBe(false);
  });
});
