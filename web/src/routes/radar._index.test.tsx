import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { delay, http, HttpResponse } from "msw";
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

// The "New" tab sends state=new; the "All" tab sends no state at all. Answering
// the All request slowly is what makes the in-between render observable.
function stubSlowAll(total = 1) {
  server.use(
    http.get("/api/radar/status", () =>
      HttpResponse.json({ last_sweep_at: "2026-05-18T11:00:00Z" }),
    ),
    http.get("/api/radar/topics", () =>
      HttpResponse.json({ items: [rawTopic(1, "Rust", 1)] }),
    ),
    http.get("/api/radar/matches", async ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      if (state === null) {
        await delay(80);
        return HttpResponse.json({ items: [rawMatch(2, "Rust")], total: 1 });
      }
      return HttpResponse.json({ items: [rawMatch(1, "Rust")], total });
    }),
  );
}

describe("RadarInboxRoute while a filter change is in flight", () => {
  it("keeps the current matches on screen instead of flashing a loader", async () => {
    stubSlowAll();
    renderAt("/radar");
    await screen.findByText("Title 1");

    await userEvent.click(screen.getByRole("button", { name: "All" }));

    expect(screen.getByText("Title 1")).toBeInTheDocument();
    expect(screen.queryByText("Loading…")).toBeNull();
    expect(await screen.findByText("Title 2")).toBeInTheDocument();
  });

  it("marks the match list busy until the new matches arrive", async () => {
    stubSlowAll();
    renderAt("/radar");
    await screen.findByText("Title 1");

    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(screen.getByTestId("pending-region")).toHaveAttribute(
      "aria-busy",
      "true",
    );

    await screen.findByText("Title 2");
    expect(screen.getByTestId("pending-region")).toHaveAttribute(
      "aria-busy",
      "false",
    );
  });

  it("hides Load more, whose page count still belongs to the old filter", async () => {
    stubSlowAll(50);
    renderAt("/radar");
    await screen.findByText("Title 1");
    expect(screen.getByRole("button", { name: /load more/i })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(screen.queryByRole("button", { name: /load more/i })).toBeNull();
  });
});

describe("RadarInboxRoute first load", () => {
  it("holds the grid open with skeleton cards", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: null }),
      ),
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(1, "Rust", 1)] }),
      ),
      http.get("/api/radar/matches", async () => {
        await delay("infinite");
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );
    renderAt("/radar");

    const cards = await screen.findAllByTestId("match-skeleton-card");
    expect(cards.length).toBeGreaterThan(0);
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("reports a failed match request instead of claiming inbox zero", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: null }),
      ),
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(1, "Rust", 1)] }),
      ),
      http.get("/api/radar/matches", () =>
        HttpResponse.json({ error: "internal", message: "boom" }, { status: 500 }),
      ),
    );
    renderAt("/radar");

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByText("Inbox zero")).toBeNull();
  });
});
