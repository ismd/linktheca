import { apiFetch } from "@/shared/api/client";
import {
  RawTopicsListSchema,
  RawTopicSchema,
  RawTopicWithStatsSchema,
  RawMatchListSchema,
  RawMatchViewSchema,
  RawMatchFindingSchema,
  RawRadarStatusSchema,
  RawTopicPreviewSchema,
  RawFeedListSchema,
  RawAddFeedResultSchema,
  RawTopicFeedbackListSchema,
  mapTopicFeedbackList,
  mapFeedListItem,
  mapTopic,
  mapTopicWithStats,
  mapMatchList,
  mapMatchView,
  mapMatchFinding,
  mapRadarStatus,
  mapTopicPreview,
} from "./schemas";
import type {
  Topic,
  TopicWithStats,
  MatchList,
  MatchView,
  MatchFinding,
  RadarStatus,
  MatchState,
  TopicPreview,
  FeedListItem,
  TopicMark,
  Verdict,
} from "./types";

export function parseInDev<T>(schema: { parse: (x: unknown) => T }, data: unknown): T {
  if (import.meta.env.DEV || import.meta.env.MODE === "test") {
    return schema.parse(data);
  }

  return data as T;
}

export async function listTopics(): Promise<TopicWithStats[]> {
  const raw = await apiFetch<unknown>(`/radar/topics`);
  const parsed = parseInDev(RawTopicsListSchema, raw);
  return parsed.items.map(mapTopicWithStats);
}

export async function getTopic(id: number): Promise<TopicWithStats> {
  const raw = await apiFetch<unknown>(`/radar/topics/${id}`);
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
}

/** Marks as the API takes them: which finding, and the verdict on it. */
function feedbackBody(marks: TopicMark[]) {
  return marks.map((m) => ({ finding_id: m.finding.id, verdict: m.verdict }));
}

export type CreateTopicInput = {
  /** Omit to let the server derive the name from the description. */
  name?: string;
  description: string;
  matchThreshold?: number;
  /** Marks given in the preview before saving. */
  marks?: TopicMark[];
};

export async function createTopic(input: CreateTopicInput): Promise<Topic> {
  const body: Record<string, unknown> = { description: input.description };
  if (input.name !== undefined) body.name = input.name;
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  if (input.marks?.length) body.feedback = feedbackBody(input.marks);
  const raw = await apiFetch<unknown>(`/radar/topics`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  return mapTopic(parseInDev(RawTopicSchema, raw));
}

export type PreviewTopicInput = {
  description: string;
  marks?: TopicMark[];
};

export async function previewTopic(input: PreviewTopicInput): Promise<TopicPreview> {
  const body: Record<string, unknown> = { description: input.description };
  if (input.marks?.length) body.feedback = feedbackBody(input.marks);
  const raw = await apiFetch<unknown>(`/radar/topics/preview`, {
    method: "POST",
    body: JSON.stringify(body),
  });

  return mapTopicPreview(parseInDev(RawTopicPreviewSchema, raw));
}

export type UpdateTopicInput = {
  name?: string;
  description?: string;
  matchThreshold?: number;
  isActive?: boolean;
  /** Replaces the topic's marks; an empty list forgets them. */
  marks?: TopicMark[];
};

export async function updateTopic(
  id: number,
  input: UpdateTopicInput,
): Promise<Topic> {
  const body: Record<string, unknown> = {};
  if (input.name !== undefined) body.name = input.name;
  if (input.description !== undefined) body.description = input.description;
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  if (input.isActive !== undefined) body.is_active = input.isActive;
  if (input.marks !== undefined) body.feedback = feedbackBody(input.marks);
  const raw = await apiFetch<unknown>(`/radar/topics/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return mapTopic(parseInDev(RawTopicSchema, raw));
}

export async function listTopicMarks(topicId: number): Promise<TopicMark[]> {
  const raw = await apiFetch<unknown>(`/radar/topics/${topicId}/feedback`);
  return mapTopicFeedbackList(parseInDev(RawTopicFeedbackListSchema, raw));
}

export async function setTopicMark(
  topicId: number,
  findingId: number,
  verdict: Verdict,
): Promise<void> {
  await apiFetch<void>(`/radar/topics/${topicId}/feedback/${findingId}`, {
    method: "PUT",
    body: JSON.stringify({ verdict }),
  });
}

export async function deleteTopicMark(topicId: number, findingId: number): Promise<void> {
  await apiFetch<void>(`/radar/topics/${topicId}/feedback/${findingId}`, {
    method: "DELETE",
  });
}

export async function deleteTopic(id: number): Promise<void> {
  await apiFetch<void>(`/radar/topics/${id}`, { method: "DELETE" });
}

export type ListMatchesArgs = {
  topicId?: number;
  state?: MatchState;
  limit: number;
  offset: number;
};

function buildMatchesQuery(args: ListMatchesArgs): string {
  const p = new URLSearchParams();
  p.set("limit", String(args.limit));
  p.set("offset", String(args.offset));
  if (args.topicId !== undefined) p.set("topic_id", String(args.topicId));
  if (args.state) p.set("state", args.state);
  return p.toString();
}

export async function listMatches(args: ListMatchesArgs): Promise<MatchList> {
  const raw = await apiFetch<unknown>(`/radar/matches?${buildMatchesQuery(args)}`);
  return mapMatchList(parseInDev(RawMatchListSchema, raw));
}

export async function getMatch(id: number): Promise<MatchView> {
  const raw = await apiFetch<unknown>(`/radar/matches/${id}`);
  return mapMatchView(parseInDev(RawMatchViewSchema, raw));
}

/** A finding from a subscribed feed, article included — what a topic preview
 * row opens, since previewed findings are not matches yet. */
export async function getFinding(id: number): Promise<MatchFinding> {
  const raw = await apiFetch<unknown>(`/radar/findings/${id}`);
  return mapMatchFinding(parseInDev(RawMatchFindingSchema, raw));
}

export async function updateMatch(
  id: number,
  input: { state: MatchState },
): Promise<void> {
  await apiFetch<void>(`/radar/matches/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ state: input.state }),
  });
}

export async function getStatus(): Promise<RadarStatus> {
  const raw = await apiFetch<unknown>(`/radar/status`);
  return mapRadarStatus(parseInDev(RawRadarStatusSchema, raw));
}

export async function listFeeds(): Promise<FeedListItem[]> {
  const raw = await apiFetch<unknown>(`/radar/feeds?limit=100`);
  const parsed = parseInDev(RawFeedListSchema, raw);
  return parsed.items.map(mapFeedListItem);
}

export async function subscribeFeed(feedId: number): Promise<void> {
  await apiFetch<void>(`/radar/subscriptions`, {
    method: "POST",
    body: JSON.stringify({ feed_id: feedId }),
  });
}

export async function unsubscribeFeed(feedId: number): Promise<void> {
  await apiFetch<void>(`/radar/subscriptions/${feedId}`, { method: "DELETE" });
}

export type AddFeedInput = { url: string; fetchIntervalSeconds: number };

// The server answers 201 for a new personal feed and 200 when the URL was
// already in the shared catalog and the caller was merely subscribed. The API
// client does not surface status codes, so the flag rides in the body.
export async function addFeed(input: AddFeedInput): Promise<{ created: boolean }> {
  const raw = await apiFetch<unknown>(`/radar/feeds`, {
    method: "POST",
    body: JSON.stringify({
      url: input.url,
      fetch_interval_seconds: input.fetchIntervalSeconds,
    }),
  });
  return { created: parseInDev(RawAddFeedResultSchema, raw).created };
}

export type UpdateFeedInput = {
  title?: string;
  fetchIntervalSeconds?: number;
  isActive?: boolean;
};

export async function updateFeed(id: number, input: UpdateFeedInput): Promise<void> {
  const body: Record<string, unknown> = {};
  if (input.title !== undefined) body.title = input.title;
  if (input.fetchIntervalSeconds !== undefined) {
    body.fetch_interval_seconds = input.fetchIntervalSeconds;
  }
  if (input.isActive !== undefined) body.is_active = input.isActive;
  await apiFetch<void>(`/radar/feeds/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function deleteFeed(id: number): Promise<void> {
  await apiFetch<void>(`/radar/feeds/${id}`, { method: "DELETE" });
}
