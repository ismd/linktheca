import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { LibraryGrid } from "./LibraryGrid";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

const rawItem = (id: number) => ({
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
});

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("LibraryGrid", () => {
  it("renders skeletons while loading", () => {
    server.use(http.get("/api/library", async () => {
      await new Promise(() => {}); // never resolve
      return HttpResponse.json({ items: [], total: 0 });
    }));
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(screen.getAllByTestId("library-skeleton-card").length).toBeGreaterThan(0);
  });

  it("renders empty state (CTA) when not filtered and no items", async () => {
    server.use(
      http.get("/api/library", () => HttpResponse.json({ items: [], total: 0 })),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(await screen.findByText(/nothing here yet/i)).toBeInTheDocument();
  });

  it("renders 'no matches' when filtered and empty", async () => {
    server.use(
      http.get("/api/library", () => HttpResponse.json({ items: [], total: 0 })),
    );
    render(<LibraryGrid filters={{ state: "read" }} />, { wrapper: wrapper() });
    expect(await screen.findByText(/no matches/i)).toBeInTheDocument();
  });

  it("renders cards and Load more when hasMore", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawItem(i + 1)),
          total: 30,
        }),
      ),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    await screen.findByText("Title 1");
    expect(screen.getByRole("button", { name: /load more/i })).toBeInTheDocument();
  });

  it("clicking Load more fetches the next page", async () => {
    let call = 0;
    server.use(
      http.get("/api/library", ({ request }) => {
        call += 1;
        const url = new URL(request.url);
        const offset = Number(url.searchParams.get("offset") ?? 0);
        return HttpResponse.json({
          items: [rawItem(offset + 1)],
          total: 2,
        });
      }),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    await screen.findByText("Title 1");
    await userEvent.click(screen.getByRole("button", { name: /load more/i }));
    await screen.findByText("Title 2");
    expect(call).toBeGreaterThanOrEqual(2);
  });

  it("shows ErrorPanel with retry on failure", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({ error: "internal", message: "boom" }, { status: 500 }),
      ),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /try again/i })).toBeInTheDocument();
  });
});
