import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import LibraryItemRoute from "./library.$id";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter initialEntries={["/library/5"]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/library/:id" element={children} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

function setScrollMetrics(scrollTop: number, scrollHeight: number, clientHeight: number) {
  for (const [prop, value] of [
    ["scrollTop", scrollTop],
    ["scrollHeight", scrollHeight],
    ["clientHeight", clientHeight],
  ] as const) {
    Object.defineProperty(document.documentElement, prop, {
      configurable: true,
      value,
    });
  }
}

const detailBody = (state: string) => ({
  id: 5,
  state,
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: state === "read" ? "2026-05-10T13:00:00Z" : null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  reading_time_seconds: 60,
  image: null,
  content: {
    id: 5,
    url: "https://example.com/x",
    text: "body text",
    fetched_at: "2026-05-10T12:00:00Z",
  },
});

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("LibraryItemRoute auto mark-as-read", () => {
  it("does not undo a manual 'Mark as unread' while scrolled to the bottom", async () => {
    // The reader is scrolled to the very end — where the action buttons live.
    setScrollMetrics(500, 1000, 500);

    let state = "read";
    const patches: unknown[] = [];
    server.use(
      http.get("/api/library/5/content", () => HttpResponse.json(detailBody(state))),
      http.patch("/api/library/5", async ({ request }) => {
        const body = (await request.json()) as { state?: string };
        patches.push(body);
        if (body.state) state = body.state;
        return HttpResponse.json(detailBody(state));
      }),
    );

    render(<LibraryItemRoute />, { wrapper: wrapper() });

    const unreadBtn = await screen.findByRole("button", { name: /mark as unread/i });
    await userEvent.click(unreadBtn);

    // It flips to the unread state (button now offers "Mark as read")...
    await screen.findByRole("button", { name: /mark as read/i });

    // ...and it must stay there. The scroll-based auto-marker must not fire
    // just because the item became unread again under the current scroll offset.
    await new Promise((r) => setTimeout(r, 100));

    expect(screen.getByRole("button", { name: /mark as read/i })).toBeInTheDocument();
    expect(patches).toEqual([{ state: "unread" }]);
  });
});