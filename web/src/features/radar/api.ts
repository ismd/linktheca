import { apiFetch } from "@/shared/api/client";
import {
  RawTopicsListSchema,
  RawTopicWithStatsSchema,
  RawMatchListSchema,
  RawMatchViewSchema,
  RawRadarStatusSchema,
  mapTopicWithStats,
  mapMatchList,
  mapMatchView,
  mapRadarStatus,
} from "./schemas";
import type {
  TopicWithStats,
  MatchList,
  MatchView,
  RadarStatus,
  MatchState,
} from "./types";

function parseInDev<T>(schema: { parse: (x: unknown) => T }, data: unknown): T {
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

export type CreateTopicInput = {
  name: string;
  description: string;
  matchThreshold?: number;
};

export async function createTopic(input: CreateTopicInput): Promise<TopicWithStats> {
  const body: Record<string, unknown> = {
    name: input.name,
    description: input.description,
  };
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  const raw = await apiFetch<unknown>(`/radar/topics`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
}

export type UpdateTopicInput = {
  name?: string;
  description?: string;
  matchThreshold?: number;
  isActive?: boolean;
};

export async function updateTopic(
  id: number,
  input: UpdateTopicInput,
): Promise<TopicWithStats> {
  const body: Record<string, unknown> = {};
  if (input.name !== undefined) body.name = input.name;
  if (input.description !== undefined) body.description = input.description;
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  if (input.isActive !== undefined) body.is_active = input.isActive;
  const raw = await apiFetch<unknown>(`/radar/topics/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
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
