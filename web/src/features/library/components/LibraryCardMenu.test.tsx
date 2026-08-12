import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { LibraryCardMenu } from "./LibraryCardMenu";
import type { LibraryItem } from "../types";

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

async function openMenu(item: LibraryItem = itemBase) {
  render(<LibraryCardMenu item={item} />, { wrapper: wrapper() });
  await userEvent.click(screen.getByRole("button", { name: /article actions/i }));
  return await screen.findByRole("menu");
}

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("LibraryCardMenu", () => {
  it("offers read, favorite, archive and delete for an unread item", async () => {
    await openMenu();

    expect(screen.getByRole("menuitem", { name: "Mark as read" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Favorite" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Archive" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
  });

  it("offers to mark a read item as unread", async () => {
    await openMenu({ ...itemBase, state: "read" });

    expect(screen.getByRole("menuitem", { name: "Mark as unread" })).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: "Mark as read" }),
    ).not.toBeInTheDocument();
  });

  it("offers to unarchive an archived item, and to mark it read", async () => {
    await openMenu({ ...itemBase, state: "archived" });

    expect(screen.getByRole("menuitem", { name: "Unarchive" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Mark as read" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Archive" })).not.toBeInTheDocument();
  });

  it("offers to unfavorite an item that is already a favorite", async () => {
    await openMenu({ ...itemBase, isFavorite: true });

    expect(screen.getByRole("menuitem", { name: "Unfavorite" })).toBeInTheDocument();
  });

  it("archives the item when Archive is chosen", async () => {
    const patch = capturePatch();
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Archive" }));

    await waitFor(() => expect(patch.body).toEqual({ state: "archived" }));
  });

  it("marks the item read when Mark as read is chosen", async () => {
    const patch = capturePatch();
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Mark as read" }));

    await waitFor(() => expect(patch.body).toEqual({ state: "read" }));
  });

  it("favorites the item when Favorite is chosen", async () => {
    const patch = capturePatch();
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Favorite" }));

    await waitFor(() => expect(patch.body).toEqual({ is_favorite: true }));
  });

  it("asks for confirmation before deleting, then deletes", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/library/5", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    const confirm = await screen.findByRole("button", { name: "Delete" });
    expect(deleted).toBe(false);

    await userEvent.click(confirm);

    await waitFor(() => expect(deleted).toBe(true));
  });

  it("keeps the item when the delete confirmation is dismissed", async () => {
    let deleted = false;
    server.use(
      http.delete("/api/library/5", () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));

    expect(deleted).toBe(false);
  });

  it("reports a failed action instead of leaving the card silently unchanged", async () => {
    server.use(
      http.patch("/api/library/5", () => new HttpResponse(null, { status: 500 })),
    );
    await openMenu();

    await userEvent.click(screen.getByRole("menuitem", { name: "Archive" }));

    expect(await screen.findByText("Couldn't archive")).toBeInTheDocument();
  });
});
