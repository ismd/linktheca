import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import SourcesRoute from "./radar.sources";

const rawFeed = (
  id: number,
  subscribed: boolean,
  over: Record<string, unknown> = {},
) => ({
  id,
  url: `https://f${id}.example/rss`,
  kind: "rss",
  title: `Feed ${id}`,
  fetch_interval_seconds: 3600,
  is_active: true,
  last_fetched_at: null,
  last_error: null,
  created_at: "2026-08-01T10:00:00Z",
  subscribed,
  finding_count: 0,
  is_own: false,
  ...over,
});

function renderAt(path: string) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/radar/sources" element={<SourcesRoute />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

function signIn(isAdmin: boolean) {
  useAuthStore.getState().setSession("t", {
    id: 1,
    email: "a@example.com",
    displayName: "A",
    isAdmin,
  });
}

beforeEach(() => {
  signIn(false);
  // The screen reads the quota off the status endpoint, and msw is configured
  // to error on unhandled requests.
  server.use(
    http.get("/api/radar/status", () =>
      HttpResponse.json({ last_sweep_at: null, max_user_feeds: 20 }),
    ),
  );
});

describe("SourcesRoute", () => {
  it("explains an empty catalog", async () => {
    server.use(
      http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
    );

    renderAt("/radar/sources");
    expect(await screen.findByText(/ask the instance admin/i)).toBeInTheDocument();
  });

  it("lists the catalog", async () => {
    server.use(
      http.get("/api/radar/feeds", () =>
        HttpResponse.json({ items: [rawFeed(3, true)], total: 1 }),
      ),
    );

    renderAt("/radar/sources");
    expect(await screen.findByRole("checkbox", { name: /feed 3/i })).toBeChecked();
  });

  it("shows the finding count in the delete confirmation", async () => {
    server.use(
      http.get("/api/radar/feeds", () =>
        HttpResponse.json({
          items: [rawFeed(3, true, { finding_count: 214, is_own: true })],
          total: 1,
        }),
      ),
    );

    renderAt("/radar/sources");

    await userEvent.click(await screen.findByRole("button", { name: /delete/i }));
    expect(await screen.findByText(/214 findings/i)).toBeInTheDocument();
  });

  it("splits personal sources from the shared catalog", async () => {
    server.use(
      http.get("/api/radar/feeds", () =>
        HttpResponse.json({
          items: [
            { ...rawFeed(1, true), title: "My Blog", is_own: true },
            { ...rawFeed(2, false), title: "The Verge", is_own: false },
          ],
          total: 2,
        }),
      ),
    );

    renderAt("/radar/sources");

    expect(await screen.findByText("My Blog")).toBeInTheDocument();
    expect(screen.getByText(/my sources/i)).toBeInTheDocument();
    expect(screen.getByText(/catalog/i)).toBeInTheDocument();

    // Only the personal row is manageable.
    expect(screen.getAllByRole("button", { name: /edit/i })).toHaveLength(1);
  });

  it("offers Add source to an ordinary user", async () => {
    server.use(
      http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
    );

    renderAt("/radar/sources");

    expect(
      await screen.findByRole("button", { name: /add source/i }),
    ).toBeInTheDocument();
  });

  it("renders the disabled screen when radar is off", async () => {
    server.use(
      http.get("/api/radar/feeds", () =>
        HttpResponse.json(
          { error: "radar_disabled", message: "radar feature is disabled on this server" },
          { status: 403 },
        ),
      ),
    );

    renderAt("/radar/sources");
    expect(await screen.findByText("Radar is disabled")).toBeInTheDocument();
  });
});
