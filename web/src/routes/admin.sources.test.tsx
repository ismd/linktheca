import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import AdminSourcesRoute from "./admin.sources";

function renderRoute() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={["/admin/sources"]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/admin/sources" element={<AdminSourcesRoute />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("t", {
    id: 1,
    email: "a@example.com",
    displayName: "A",
    isAdmin: true,
  });
});

describe("AdminSourcesRoute", () => {
  it("manages catalog rows and offers no subscription checkbox", async () => {
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

    renderRoute();

    expect(await screen.findByText("The Verge")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("prompts to add the first feed when the catalog is empty", async () => {
    server.use(
      http.get("/api/admin/radar/feeds", () =>
        HttpResponse.json({ items: [], total: 0 }),
      ),
    );

    renderRoute();

    expect(
      await screen.findByText(/add the first shared feed/i),
    ).toBeInTheDocument();
  });

  it("renders the disabled screen when radar is off", async () => {
    server.use(
      http.get("/api/admin/radar/feeds", () =>
        HttpResponse.json(
          { error: "radar_disabled", message: "radar feature is disabled on this server" },
          { status: 403 },
        ),
      ),
    );

    renderRoute();

    expect(await screen.findByText("Radar is disabled")).toBeInTheDocument();
  });
});
