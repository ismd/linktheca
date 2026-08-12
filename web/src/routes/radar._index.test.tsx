import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import RadarInboxRoute from "./radar._index";

const rawTopic = (id: number, name: string, newCount: number) => ({
  id,
  user_id: 1,
  name,
  description: "D",
  match_threshold: 0.55,
  is_active: true,
  has_embedding: true,
  created_at: "2026-05-01T10:00:00Z",
  updated_at: "2026-05-02T10:00:00Z",
  stats: {
    new_count: newCount,
    total_count: 10,
    source_count: 2,
    last_match_at: null,
  },
});

const rawMatch = (id: number, topicName: string) => ({
  id,
  topic_id: 1,
  topic_name: topicName,
  similarity: 0.7,
  state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: id + 100,
    feed_id: 5,
    feed_title: "Ink & Switch",
    url: `https://example.com/${id}`,
    title: `Title ${id}`,
    summary: null,
    published_at: null,
    discovered_at: "2026-05-18T09:00:00Z",
  },
});

type Scenario = {
  topics?: unknown[];
  matches?: unknown[];
  topicsError?: { status: number; body: Record<string, unknown> };
};

const seen: string[] = [];

function firstMatchesUrl() {
  const url = seen[0];
  if (!url) throw new Error("no request to /api/radar/matches was made");
  return new URL(url);
}

function stub(s: Scenario) {
  server.use(
    http.get("/api/radar/status", () =>
      HttpResponse.json({ last_sweep_at: "2026-05-18T11:00:00Z" }),
    ),
    http.get("/api/radar/topics", () =>
      s.topicsError
        ? HttpResponse.json(s.topicsError.body, { status: s.topicsError.status })
        : HttpResponse.json({ items: s.topics ?? [] }),
    ),
    http.get("/api/radar/matches", ({ request }) => {
      seen.push(request.url);
      return HttpResponse.json({
        items: s.matches ?? [],
        total: (s.matches ?? []).length,
      });
    }),
  );
}

function renderAt(path: string) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/radar" element={<RadarInboxRoute />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  seen.length = 0;
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("RadarInboxRoute", () => {
  it("requests unread matches across all topics by default", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    const url = firstMatchesUrl();
    expect(url.searchParams.get("state")).toBe("new");
    expect(url.searchParams.get("topic_id")).toBeNull();
  });

  it("stamps each card with its topic name", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    const card = await screen.findByRole("link", { name: /Title 1/ });
    expect(within(card).getByText("Rust")).toBeInTheDocument();
  });

  it("drops the state parameter when ?state=all", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?state=all");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(firstMatchesUrl().searchParams.get("state")).toBeNull();
  });

  it("sends topic_id when ?topic is set", async () => {
    stub({ topics: [rawTopic(3, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?topic=3");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(firstMatchesUrl().searchParams.get("topic_id")).toBe("3");
  });

  it("ignores a non-numeric topic parameter", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?topic=abc");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(firstMatchesUrl().searchParams.get("topic_id")).toBeNull();
  });

  it("prompts to create a topic when there are none", async () => {
    stub({ topics: [], matches: [] });
    renderAt("/radar");

    expect(await screen.findByText("Nothing on your radar yet")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "All topics" })).toBeNull();
  });

  it("shows inbox zero when nothing is unread", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [] });
    renderAt("/radar");

    expect(await screen.findByText("Inbox zero")).toBeInTheDocument();
  });

  it("shows the standing-watch empty state when All is empty", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [] });
    renderAt("/radar?state=all");

    expect(await screen.findByText("Nothing yet")).toBeInTheDocument();
    expect(screen.queryByText("Inbox zero")).toBeNull();
  });

  it("renders the disabled screen when radar is off", async () => {
    stub({
      topicsError: {
        status: 403,
        body: {
          error: "radar_disabled",
          message: "radar feature is disabled on this server",
        },
      },
    });
    renderAt("/radar");

    expect(await screen.findByText("Radar is disabled")).toBeInTheDocument();
  });

  it("links to the topics page from the header", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    expect(await screen.findByRole("link", { name: /topics/i })).toHaveAttribute(
      "href",
      "/radar/topics",
    );
  });
});
