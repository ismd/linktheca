import { describe, it, expect } from "vitest";
import {
  RawTopicWithStatsSchema,
  RawMatchViewSchema,
  RawMatchListSchema,
  RawRadarStatusSchema,
  mapTopicWithStats,
  mapMatchView,
  mapMatchList,
  mapRadarStatus,
} from "./schemas";

describe("radar schemas", () => {
  it("mapTopicWithStats converts snake_case + dates", () => {
    const raw = RawTopicWithStatsSchema.parse({
      id: 1,
      user_id: 7,
      name: "Topic",
      description: "Desc",
      match_threshold: 0.55,
      is_active: true,
      has_embedding: true,
      created_at: "2026-05-01T10:00:00Z",
      updated_at: "2026-05-02T10:00:00Z",
      stats: {
        new_count: 3,
        total_count: 10,
        source_count: 2,
        last_match_at: "2026-05-18T09:00:00Z",
      },
    });
    const t = mapTopicWithStats(raw);
    expect(t.id).toBe(1);
    expect(t.userId).toBe(7);
    expect(t.matchThreshold).toBe(0.55);
    expect(t.isActive).toBe(true);
    expect(t.hasEmbedding).toBe(true);
    expect(t.createdAt).toBeInstanceOf(Date);
    expect(t.stats.newCount).toBe(3);
    expect(t.stats.totalCount).toBe(10);
    expect(t.stats.sourceCount).toBe(2);
    expect(t.stats.lastMatchAt).toBeInstanceOf(Date);
  });

  it("mapTopicWithStats handles null last_match_at", () => {
    const raw = RawTopicWithStatsSchema.parse({
      id: 1, user_id: 7, name: "T", description: "D",
      match_threshold: 0.55, is_active: true, has_embedding: false,
      created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
      stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
    });
    expect(mapTopicWithStats(raw).stats.lastMatchAt).toBeNull();
  });

  it("mapMatchView converts nested finding + dates", () => {
    const raw = RawMatchViewSchema.parse({
      id: 42, topic_id: 7, topic_name: "T",
      similarity: 0.7, state: "new",
      matched_at: "2026-05-18T10:00:00Z",
      finding: {
        id: 100, feed_id: 5, feed_title: "Feed",
        url: "https://x.example/a", title: "Title", summary: "Summary",
        published_at: "2026-05-17T10:00:00Z",
        discovered_at: "2026-05-18T09:00:00Z",
      },
    });
    const m = mapMatchView(raw);
    expect(m.id).toBe(42);
    expect(m.topicName).toBe("T");
    expect(m.matchedAt).toBeInstanceOf(Date);
    expect(m.finding.feedTitle).toBe("Feed");
    expect(m.finding.publishedAt).toBeInstanceOf(Date);
    expect(m.finding.discoveredAt).toBeInstanceOf(Date);
  });

  it("mapMatchList maps items + total", () => {
    const raw = RawMatchListSchema.parse({
      items: [],
      total: 0,
    });
    const list = mapMatchList(raw);
    expect(list.total).toBe(0);
    expect(list.items).toEqual([]);
  });

  it("mapRadarStatus handles null", () => {
    expect(mapRadarStatus(RawRadarStatusSchema.parse({ last_sweep_at: null }))
      .lastSweepAt).toBeNull();
    const filled = mapRadarStatus(RawRadarStatusSchema.parse({
      last_sweep_at: "2026-05-18T10:00:00Z",
    }));
    expect(filled.lastSweepAt).toBeInstanceOf(Date);
  });
});
