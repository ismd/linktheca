import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import SourcesRoute from "./radar.sources";

const rawFeed = (id: number, subscribed: boolean) => ({
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
});

describe("SourcesRoute", () => {
  it("shows a different empty state for admins", async () => {
    server.use(
      http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
    );

    renderAt("/radar/sources");
    expect(await screen.findByText(/ask the instance admin/i)).toBeInTheDocument();
  });

  it("prompts an admin to add the first feed", async () => {
    server.use(
      http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
    );

    signIn(true);
    renderAt("/radar/sources");
    expect(await screen.findByText(/add the first feed/i)).toBeInTheDocument();
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
