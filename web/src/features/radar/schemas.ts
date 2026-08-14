import { z } from "zod";
import type {
  Topic,
  TopicWithStats,
  MatchFinding,
  MatchView,
  MatchList,
  RadarStatus,
  TopicPreview,
} from "./types";

export const RawTopicStatsSchema = z.object({
  new_count: z.number().int(),
  total_count: z.number().int(),
  source_count: z.number().int(),
  last_match_at: z.string().nullable(),
});

export const RawTopicSchema = z.object({
  id: z.number().int(),
  user_id: z.number().int(),
  name: z.string(),
  description: z.string(),
  match_threshold: z.number(),
  is_active: z.boolean(),
  has_embedding: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const RawTopicWithStatsSchema = RawTopicSchema.extend({
  stats: RawTopicStatsSchema,
});

export const RawMatchFindingSchema = z.object({
  id: z.number().int(),
  feed_id: z.number().int(),
  feed_title: z.string().nullable(),
  url: z.string(),
  title: z.string().nullable(),
  summary: z.string().nullable(),
  published_at: z.string().nullable(),
  discovered_at: z.string(),
});

export const RawMatchViewSchema = z.object({
  id: z.number().int(),
  topic_id: z.number().int(),
  topic_name: z.string(),
  similarity: z.number(),
  state: z.enum(["new", "seen"]),
  matched_at: z.string(),
  finding: RawMatchFindingSchema,
});

export const RawMatchListSchema = z.object({
  items: z.array(RawMatchViewSchema),
  total: z.number().int(),
});

export const RawPreviewMatchSchema = z.object({
  similarity: z.number(),
  finding: RawMatchFindingSchema,
});

export const RawTopicPreviewSchema = z.object({
  items: z.array(RawPreviewMatchSchema),
  threshold: z.number(),
});

export const RawTopicsListSchema = z.object({
  items: z.array(RawTopicWithStatsSchema),
});

export const RawRadarStatusSchema = z.object({
  last_sweep_at: z.string().nullable(),
});

export type RawTopic = z.infer<typeof RawTopicSchema>;
export type RawTopicWithStats = z.infer<typeof RawTopicWithStatsSchema>;
export type RawMatchView = z.infer<typeof RawMatchViewSchema>;

export function mapTopic(raw: RawTopic): Topic {
  return {
    id: raw.id,
    userId: raw.user_id,
    name: raw.name,
    description: raw.description,
    matchThreshold: raw.match_threshold,
    isActive: raw.is_active,
    hasEmbedding: raw.has_embedding,
    createdAt: new Date(raw.created_at),
    updatedAt: new Date(raw.updated_at),
  };
}

export function mapTopicWithStats(raw: RawTopicWithStats): TopicWithStats {
  return {
    ...mapTopic(raw),
    stats: {
      newCount: raw.stats.new_count,
      totalCount: raw.stats.total_count,
      sourceCount: raw.stats.source_count,
      lastMatchAt: raw.stats.last_match_at ? new Date(raw.stats.last_match_at) : null,
    },
  };
}

function mapMatchFinding(raw: z.infer<typeof RawMatchFindingSchema>): MatchFinding {
  return {
    id: raw.id,
    feedId: raw.feed_id,
    feedTitle: raw.feed_title,
    url: raw.url,
    title: raw.title,
    summary: raw.summary,
    publishedAt: raw.published_at ? new Date(raw.published_at) : null,
    discoveredAt: new Date(raw.discovered_at),
  };
}

export function mapMatchView(raw: RawMatchView): MatchView {
  return {
    id: raw.id,
    topicId: raw.topic_id,
    topicName: raw.topic_name,
    similarity: raw.similarity,
    state: raw.state,
    matchedAt: new Date(raw.matched_at),
    finding: mapMatchFinding(raw.finding),
  };
}

export function mapTopicPreview(
  raw: z.infer<typeof RawTopicPreviewSchema>,
): TopicPreview {
  return {
    items: raw.items.map((i) => ({
      similarity: i.similarity,
      finding: mapMatchFinding(i.finding),
    })),
    threshold: raw.threshold,
  };
}

export function mapMatchList(raw: z.infer<typeof RawMatchListSchema>): MatchList {
  return {
    items: raw.items.map(mapMatchView),
    total: raw.total,
  };
}

export function mapRadarStatus(raw: z.infer<typeof RawRadarStatusSchema>): RadarStatus {
  return {
    lastSweepAt: raw.last_sweep_at ? new Date(raw.last_sweep_at) : null,
  };
}
