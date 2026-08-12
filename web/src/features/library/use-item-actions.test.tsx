import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { useItemActions } from "./use-item-actions";
import type { LibraryItem, LibraryState } from "./types";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

const itemBase: LibraryItem = {
  id: 5,
  state: "unread",
  isFavorite: false,
  note: null,
  savedAt: new Date("2026-05-10T12:00:00Z"),
  readAt: null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  readingTimeSeconds: 60,
  image: null,
};

const rawItem = (overrides: Record<string, unknown> = {}) => ({
  id: 5,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  reading_time_seconds: 60,
  ...overrides,
});

function capturePatch() {
  const box: { body: unknown } = { body: null };
  server.use(
    http.patch("/api/library/5", async ({ request }) => {
      box.body = await request.json();
      return HttpResponse.json(rawItem());
    }),
  );
  return box;
}

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("useItemActions", () => {
  it("toggleRead marks an unread item as read", async () => {
    const patch = capturePatch();
    const { result } = renderHook(() => useItemActions(itemBase), {
      wrapper: wrapper(),
    });

    act(() => result.current.toggleRead());

    await waitFor(() => expect(patch.body).toEqual({ state: "read" }));
  });

  it("toggleRead marks a read item as unread", async () => {
    const patch = capturePatch();
    const { result } = renderHook(
      () => useItemActions({ ...itemBase, state: "read" }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleRead());

    await waitFor(() => expect(patch.body).toEqual({ state: "unread" }));
  });

  it("toggleRead marks an archived item as read", async () => {
    const patch = capturePatch();
    const { result } = renderHook(
      () => useItemActions({ ...itemBase, state: "archived" }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleRead());

    await waitFor(() => expect(patch.body).toEqual({ state: "read" }));
  });

  it("notifies the caller before a manual read toggle so auto-marking can stand down", async () => {
    capturePatch();
    const onReadStateToggled = vi.fn();
    const { result } = renderHook(
      () => useItemActions(itemBase, { onReadStateToggled }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleRead());

    expect(onReadStateToggled).toHaveBeenCalledTimes(1);
  });

  it("toggleArchive archives a non-archived item", async () => {
    const patch = capturePatch();
    const { result } = renderHook(
      () => useItemActions({ ...itemBase, state: "read" }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleArchive());

    await waitFor(() => expect(patch.body).toEqual({ state: "archived" }));
  });

  it("toggleArchive returns an archived item to unread", async () => {
    const patch = capturePatch();
    const { result } = renderHook(
      () => useItemActions({ ...itemBase, state: "archived" }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleArchive());

    await waitFor(() => expect(patch.body).toEqual({ state: "unread" }));
  });

  it("toggleFavorite flips the favorite flag", async () => {
    const patch = capturePatch();
    const { result } = renderHook(
      () => useItemActions({ ...itemBase, isFavorite: true }),
      { wrapper: wrapper() },
    );

    act(() => result.current.toggleFavorite());

    await waitFor(() => expect(patch.body).toEqual({ is_favorite: false }));
  });

  it("confirms deletion, then reports it to the caller", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/library/5", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const onDeleted = vi.fn();
    const { result } = renderHook(() => useItemActions(itemBase, { onDeleted }), {
      wrapper: wrapper(),
    });

    act(() => result.current.requestDelete());
    expect(result.current.confirmOpen).toBe(true);

    act(() => result.current.confirmDelete());

    await waitFor(() => expect(deleted).toBe(true));
    await waitFor(() => expect(onDeleted).toHaveBeenCalledTimes(1));
    expect(result.current.confirmOpen).toBe(false);
  });

  it.each<[LibraryState, string]>([
    ["unread", "Archived"],
    ["archived", "Moved to unread"],
  ])("announces the archive toggle from %s state", async (state, message) => {
    capturePatch();
    const { result } = renderHook(() => useItemActions({ ...itemBase, state }), {
      wrapper: wrapper(),
    });

    act(() => result.current.toggleArchive());

    expect(await screen.findByText(message)).toBeInTheDocument();
  });

  it("announces a read toggle so a card leaving a filtered list is explained", async () => {
    capturePatch();
    const { result } = renderHook(() => useItemActions(itemBase), {
      wrapper: wrapper(),
    });

    act(() => result.current.toggleRead());

    expect(await screen.findByText("Marked as read")).toBeInTheDocument();
  });

  it("stays quiet on favorite, which the card already shows with a stamp", async () => {
    const patch = capturePatch();
    const { result } = renderHook(() => useItemActions(itemBase), {
      wrapper: wrapper(),
    });

    act(() => result.current.toggleFavorite());

    await waitFor(() => expect(patch.body).toEqual({ is_favorite: true }));
    expect(screen.queryByText(/favorite/i)).not.toBeInTheDocument();
  });

  it("surfaces a failed update instead of leaving the card silently unchanged", async () => {
    server.use(
      http.patch("/api/library/5", () => new HttpResponse(null, { status: 500 })),
    );
    const { result } = renderHook(() => useItemActions(itemBase), {
      wrapper: wrapper(),
    });

    act(() => result.current.toggleArchive());

    expect(await screen.findByText("Couldn't archive")).toBeInTheDocument();
  });
});
