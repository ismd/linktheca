import { z } from "zod";
import type {
  Topic,
  TopicWithStats,
  MatchFinding,
  MatchView,
  MatchList,
  RadarStatus,
  TopicPreview,
  FeedListItem,
  TopicMark,
} from "./types";

export const RawTopicStatsSchema = z.object({
  new_count: z.number().int(),
  total_count: z.number().int(),
  source_count: z.number().int(),
  last_match_at: z.string().nullable(),
  marked_relevant: z.number().int().optional(),
  marked_irrelevant: z.number().int().optional(),
});

export const RawTopicSchema = z.object({
  id: z.number().int(),
  user_id: z.number().int(),
  name: z.string(),
  description: z.string(),
  match_threshold: z.number(),
  match_cutoff: z.number().optional(),
  is_active: z.boolean(),
  has_embedding: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const RawTopicWithStatsSchema = RawTopicSchema.extend({
  stats: RawTopicStatsSchema,
});

export const RawFindingContentSchema = z.object({
  html: z.string().nullable(),
  text: z.string().nullable(),
  byline: z.string().nullable(),
  site_name: z.string().nullable(),
  reading_time_seconds: z.number().int().nullable(),
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
  content: RawFindingContentSchema.nullish(),
});

export const RawMatchViewSchema = z.object({
  id: z.number().int(),
  topic_id: z.number().int(),
  topic_name: z.string(),
  similarity: z.number(),
  state: z.enum(["new", "seen"]),
  matched_at: z.string(),
  verdict: z.enum(["relevant", "irrelevant"]).nullish(),
  finding: RawMatchFindingSchema,
});

export const RawTopicFeedbackListSchema = z.object({
  items: z.array(
    z.object({
      verdict: z.enum(["relevant", "irrelevant"]),
      created_at: z.string(),
      finding: RawMatchFindingSchema,
    }),
  ),
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
  max_user_feeds: z.number().int().optional(),
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
    matchCutoff: raw.match_cutoff ?? raw.match_threshold,
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
      markedRelevant: raw.stats.marked_relevant ?? 0,
      markedIrrelevant: raw.stats.marked_irrelevant ?? 0,
    },
  };
}

export function mapMatchFinding(raw: z.infer<typeof RawMatchFindingSchema>): MatchFinding {
  return {
    id: raw.id,
    feedId: raw.feed_id,
    feedTitle: raw.feed_title,
    url: raw.url,
    title: raw.title,
    summary: raw.summary,
    publishedAt: raw.published_at ? new Date(raw.published_at) : null,
    discoveredAt: new Date(raw.discovered_at),
    content: raw.content
      ? {
          html: raw.content.html,
          text: raw.content.text,
          byline: raw.content.byline,
          siteName: raw.content.site_name,
          readingTimeSeconds: raw.content.reading_time_seconds,
        }
      : null,
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
    verdict: raw.verdict ?? null,
    finding: mapMatchFinding(raw.finding),
  };
}

export function mapTopicFeedbackList(
  raw: z.infer<typeof RawTopicFeedbackListSchema>,
): TopicMark[] {
  return raw.items.map((i) => ({
    verdict: i.verdict,
    finding: mapMatchFinding(i.finding),
  }));
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
    maxUserFeeds: raw.max_user_feeds ?? null,
  };
}

export const RawFeedSchema = z.object({
  id: z.number().int(),
  url: z.string(),
  kind: z.string(),
  title: z.string().nullable().optional(),
  fetch_interval_seconds: z.number().int(),
  is_active: z.boolean(),
  last_fetched_at: z.string().nullable().optional(),
  last_error: z.string().nullable().optional(),
  created_at: z.string(),
});

export const RawFeedListItemSchema = RawFeedSchema.extend({
  subscribed: z.boolean(),
  finding_count: z.number().int(),
  is_own: z.boolean(),
});

export const RawAddFeedResultSchema = z.object({
  feed: RawFeedSchema,
  created: z.boolean(),
});

export const RawFeedListSchema = z.object({
  items: z.array(RawFeedListItemSchema),
  total: z.number().int(),
});

export function mapFeedListItem(
  raw: z.infer<typeof RawFeedListItemSchema>,
): FeedListItem {
  return {
    id: raw.id,
    url: raw.url,
    kind: raw.kind,
    title: raw.title ?? null,
    fetchIntervalSeconds: raw.fetch_interval_seconds,
    isActive: raw.is_active,
    lastFetchedAt: raw.last_fetched_at ? new Date(raw.last_fetched_at) : null,
    lastError: raw.last_error ?? null,
    createdAt: new Date(raw.created_at),
    subscribed: raw.subscribed,
    findingCount: raw.finding_count,
    isOwn: raw.is_own,
  };
}
