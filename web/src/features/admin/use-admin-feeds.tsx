import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  listGlobalFeeds,
  addGlobalFeed,
  updateGlobalFeed,
  deleteGlobalFeed,
} from "./api";
import { radarKeys } from "@/features/radar/use-radar";
import type { AddFeedInput, UpdateFeedInput } from "@/features/radar/api";

export const adminKeys = {
  all: ["admin"] as const,
  feeds: ["admin", "feeds"] as const,
};

export function useGlobalFeedsQuery() {
  return useQuery({ queryKey: adminKeys.feeds, queryFn: listGlobalFeeds });
}

// Catalog mutations invalidate the Radar feed list too: a deleted or paused
// global feed must disappear from the Radar screen, not only from Admin.
export function useAddGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddFeedInput) => addGlobalFeed(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.feeds });
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useUpdateGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateFeedInput }) =>
      updateGlobalFeed(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.feeds });
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useDeleteGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteGlobalFeed(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.feeds });
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}
