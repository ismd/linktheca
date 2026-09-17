import { apiFetch } from "@/shared/api/client";
import { parseInDev } from "@/features/radar/api";
import { RawFeedListSchema, mapFeedListItem } from "@/features/radar/schemas";
import type { FeedListItem } from "@/features/radar/types";
import type { AddFeedInput, UpdateFeedInput } from "@/features/radar/api";

// The admin catalog is Radar data, so it reuses Radar's wire schemas; only the
// endpoints differ.
export async function listGlobalFeeds(): Promise<FeedListItem[]> {
  const raw = await apiFetch<unknown>(`/admin/radar/feeds?limit=100`);
  return parseInDev(RawFeedListSchema, raw).items.map(mapFeedListItem);
}

export async function addGlobalFeed(input: AddFeedInput): Promise<void> {
  await apiFetch<void>(`/admin/radar/feeds`, {
    method: "POST",
    body: JSON.stringify({
      url: input.url,
      fetch_interval_seconds: input.fetchIntervalSeconds,
    }),
  });
}

export async function updateGlobalFeed(id: number, input: UpdateFeedInput): Promise<void> {
  const body: Record<string, unknown> = {};
  if (input.title !== undefined) body.title = input.title;
  if (input.fetchIntervalSeconds !== undefined) {
    body.fetch_interval_seconds = input.fetchIntervalSeconds;
  }
  if (input.isActive !== undefined) body.is_active = input.isActive;
  await apiFetch<void>(`/admin/radar/feeds/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function deleteGlobalFeed(id: number): Promise<void> {
  await apiFetch<void>(`/admin/radar/feeds/${id}`, { method: "DELETE" });
}
