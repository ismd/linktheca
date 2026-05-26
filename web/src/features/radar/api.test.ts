import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  listTopics,
  getTopic,
  createTopic,
  updateTopic,
  deleteTopic,
  listMatches,
  getMatch,
  updateMatch,
  getStatus,
} from "./api";

const rawTopic = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  user_id: 7,
  name: "T",
  description: "Desc",
  match_threshold: 0.55,
  is_active: true,
  has_embedding: true,
  created_at: "2026-05-01T10:00:00Z",
  updated_at: "2026-05-02T10:00:00Z",
  stats: {
    new_count: 0,
    total_count: 0,
    source_count: 0,
    last_match_at: null,
  },
  ...overrides,
});

const rawMatch = (overrides: Record<string, unknown> = {}) => ({
  id: 100,
  topic_id: 1,
  topic_name: "T",
  similarity: 0.7,
  state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 200, feed_id: 5, feed_title: "Feed",
    url: "https://x.example/a", title: "Title", summary: "Summary",
    published_at: "2026-05-17T10:00:00Z",
    discovered_at: "2026-05-18T09:00:00Z",
  },
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("radar api", () => {
  it("listTopics maps items[]", async () => {
    server.use(
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(), rawTopic({ id: 2 })] })),
    );
    const ts = await listTopics();
    expect(ts).toHaveLength(2);
    expect(ts[0]!.createdAt).toBeInstanceOf(Date);
    expect(ts[0]!.stats.lastMatchAt).toBeNull();
  });

  it("getTopic maps response", async () => {
    server.use(
      http.get("/api/radar/topics/42", () =>
        HttpResponse.json(rawTopic({ id: 42 }))),
    );
    const t = await getTopic(42);
    expect(t.id).toBe(42);
  });

  it("createTopic POSTs body", async () => {
    let captured: unknown = null;
    server.use(
      http.post("/api/radar/topics", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawTopic({ id: 99, name: "New" }), { status: 201 });
      }),
    );
    const t = await createTopic({ name: "New", description: "Desc longer than 10 chars" });
    expect(captured).toEqual({ name: "New", description: "Desc longer than 10 chars" });
    expect(t.id).toBe(99);
  });

  it("updateTopic PATCHes only provided fields", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/radar/topics/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawTopic({ id: 3, is_active: false }));
      }),
    );
    await updateTopic(3, { isActive: false });
    expect(captured).toEqual({ is_active: false });
  });

  it("deleteTopic DELETEs", async () => {
    let called = false;
    server.use(
      http.delete("/api/radar/topics/9", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await deleteTopic(9);
    expect(called).toBe(true);
  });

  it("listMatches sends query params", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [rawMatch()], total: 1 });
      }),
    );
    const page = await listMatches({ topicId: 5, state: "new", limit: 20, offset: 40 });
    expect(capturedUrl).toContain("topic_id=5");
    expect(capturedUrl).toContain("state=new");
    expect(capturedUrl).toContain("limit=20");
    expect(capturedUrl).toContain("offset=40");
    expect(page.total).toBe(1);
  });

  it("listMatches omits filters when not set", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );
    await listMatches({ limit: 20, offset: 0 });
    expect(capturedUrl).not.toContain("topic_id=");
    expect(capturedUrl).not.toContain("state=");
  });

  it("getMatch maps response", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch({ id: 42 }))),
    );
    const m = await getMatch(42);
    expect(m.id).toBe(42);
    expect(m.matchedAt).toBeInstanceOf(Date);
  });

  it("updateMatch PATCHes state and returns 204", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/radar/matches/42", async ({ request }) => {
        captured = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await updateMatch(42, { state: "seen" });
    expect(captured).toEqual({ state: "seen" });
  });

  it("getStatus maps response", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: "2026-05-18T10:00:00Z" })),
    );
    const s = await getStatus();
    expect(s.lastSweepAt).toBeInstanceOf(Date);
  });
});
