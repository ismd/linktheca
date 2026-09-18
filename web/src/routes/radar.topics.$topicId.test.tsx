import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { delay, http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import TopicRoute from "./radar.topics.$topicId";

const rawTopic = {
  id: 7,
  user_id: 1,
  name: "Rust",
  description: "D",
  match_threshold: 0.55,
  is_active: true,
  has_embedding: true,
  created_at: "2026-05-01T10:00:00Z",
  updated_at: "2026-05-02T10:00:00Z",
  stats: {
    new_count: 1,
    total_count: 10,
    source_count: 2,
    last_match_at: null,
  },
};

function renderTopic() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={["/radar/topics/7"]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/radar/topics/:topicId" element={<TopicRoute />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("TopicRoute match list", () => {
  it("holds the grid open with skeleton cards while matches load", async () => {
    server.use(
      http.get("/api/radar/topics/7", () => HttpResponse.json(rawTopic)),
      http.get("/api/radar/matches", async () => {
        await delay("infinite");
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );
    renderTopic();

    const cards = await screen.findAllByTestId("match-skeleton-card");
    expect(cards.length).toBeGreaterThan(0);
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("reports a failed match request instead of claiming the topic is empty", async () => {
    server.use(
      http.get("/api/radar/topics/7", () => HttpResponse.json(rawTopic)),
      http.get("/api/radar/matches", () =>
        HttpResponse.json({ error: "internal", message: "boom" }, { status: 500 }),
      ),
    );
    renderTopic();

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByText("Nothing yet")).toBeNull();
  });
});
