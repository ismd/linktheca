import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { libraryKeys } from "./use-library";
import {
  useSaveLink,
  useUpdateItem,
  useDeleteItem,
} from "./use-mutations";
import type { ListPage } from "./types";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper };
}

const rawItem = (id: number, overrides: Record<string, unknown> = {}) => ({
  id,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: `https://example.com/${id}`,
  title: `Title ${id}`,
  excerpt: "ex",
  reading_time_seconds: 120,
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("useSaveLink", () => {
  it("on success invalidates list query", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json(rawItem(7), { status: 201 }),
      ),
    );

    const { qc, wrapper } = makeWrapper();
    // seed list cache
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      { pages: [{ items: [], total: 0 }], pageParams: [0] },
    );

    const { result } = renderHook(() => useSaveLink(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync("https://example.com/7");
    });

    await waitFor(() =>
      expect(qc.getQueryState(libraryKeys.list({}))?.isInvalidated).toBe(true),
    );
  });
});

describe("useUpdateItem (optimistic favorite)", () => {
  it("optimistically updates list cache on isFavorite toggle", async () => {
    let resolve!: (v: Response) => void;
    const slow = new Promise<Response>((r) => (resolve = r));
    server.use(http.patch("/api/library/1", () => slow));

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      {
        pages: [
          {
            items: [
              {
                id: 1,
                state: "unread",
                isFavorite: false,
                note: null,
                savedAt: new Date(),
                readAt: null,
                url: "u",
                title: "t",
                excerpt: "e",
                readingTimeSeconds: 60,
              },
            ],
            total: 1,
          },
        ],
        pageParams: [0],
      },
    );

    const { result } = renderHook(() => useUpdateItem(), { wrapper });

    act(() => {
      result.current.mutate({ id: 1, input: { isFavorite: true } });
    });

    await waitFor(() => {
      const data = qc.getQueryData<{ pages: ListPage[] }>(libraryKeys.list({}));
      expect(data!.pages[0]!.items[0]!.isFavorite).toBe(true);
    });

    // resolve to make mutation finish
    resolve(
      HttpResponse.json(rawItem(1, { is_favorite: true })) as unknown as Response,
    );
  });

  it("rolls back optimistic update on failure", async () => {
    server.use(
      http.patch("/api/library/1", () =>
        HttpResponse.json({ error: "internal", message: "boom" }, { status: 500 }),
      ),
    );

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      {
        pages: [
          {
            items: [
              {
                id: 1,
                state: "unread",
                isFavorite: false,
                note: null,
                savedAt: new Date(),
                readAt: null,
                url: "u",
                title: "t",
                excerpt: "e",
                readingTimeSeconds: 60,
              },
            ],
            total: 1,
          },
        ],
        pageParams: [0],
      },
    );

    const { result } = renderHook(() => useUpdateItem(), { wrapper });

    await act(async () => {
      try {
        await result.current.mutateAsync({ id: 1, input: { isFavorite: true } });
      } catch {
        // expected
      }
    });

    const data = qc.getQueryData<{ pages: ListPage[] }>(libraryKeys.list({}));
    expect(data!.pages[0]!.items[0]!.isFavorite).toBe(false);
  });
});

describe("useDeleteItem", () => {
  it("invalidates list and removes detail cache", async () => {
    server.use(
      http.delete("/api/library/5", () => new HttpResponse(null, { status: 204 })),
    );

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(libraryKeys.detail(5), { id: 5 });
    qc.setQueryData(libraryKeys.list({}), {
      pages: [{ items: [], total: 0 }],
      pageParams: [0],
    });

    const { result } = renderHook(() => useDeleteItem(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(5);
    });

    expect(qc.getQueryData(libraryKeys.detail(5))).toBeUndefined();
    expect(qc.getQueryState(libraryKeys.list({}))?.isInvalidated).toBe(true);
  });
});
