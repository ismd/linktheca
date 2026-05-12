import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { ReaderActions } from "./ReaderActions";
import type { LibraryItem } from "../types";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        {children}
        <Toaster />
      </QueryClientProvider>
    </MemoryRouter>
  );
}

const itemBase: LibraryItem = {
  id: 5,
  state: "unread",
  isFavorite: false,
  note: null,
  savedAt: new Date(),
  readAt: null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  readingTimeSeconds: 60,
};

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

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

describe("ReaderActions", () => {
  it("toggle favorite calls PATCH with is_favorite=true", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ is_favorite: true }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /favorite/i }));
    await waitFor(() =>
      expect(captured).toEqual({ is_favorite: true }),
    );
  });

  it("mark-read button calls PATCH with state=read when unread", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "read" }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /mark as read/i }));
    await waitFor(() => expect(captured).toEqual({ state: "read" }));
  });

  it("mark-unread button shows when state=read and PATCHes back to unread", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "unread" }));
      }),
    );
    render(<ReaderActions item={{ ...itemBase, state: "read" }} />, {
      wrapper: wrapper(),
    });
    await userEvent.click(screen.getByRole("button", { name: /mark as unread/i }));
    await waitFor(() => expect(captured).toEqual({ state: "unread" }));
  });

  it("archive button calls PATCH with state=archived", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "archived" }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /archive/i }));
    await waitFor(() => expect(captured).toEqual({ state: "archived" }));
  });

  it("delete opens AlertDialog and on confirm calls DELETE then navigates", async () => {
    let called = false;
    server.use(
      http.delete("/api/library/5", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const onDeleted = vi.fn();
    render(<ReaderActions item={itemBase} onDeleted={onDeleted} />, {
      wrapper: wrapper(),
    });
    await userEvent.click(screen.getByRole("button", { name: /delete/i }));
    const confirm = await screen.findByRole("button", { name: "Delete" });
    await userEvent.click(confirm);
    await waitFor(() => expect(called).toBe(true));
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("open original opens external link in new tab", () => {
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    const link = screen.getByRole("link", { name: /open original/i });
    expect(link).toHaveAttribute("href", "https://example.com/x");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });
});
