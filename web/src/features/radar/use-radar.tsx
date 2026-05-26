import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  listTopics,
  getTopic,
  listMatches,
  getMatch,
  getStatus,
} from "./api";
import { PAGE_SIZE, type MatchFilters, type MatchList } from "./types";

export const radarKeys = {
  all: ["radar"] as const,
  topics: ["radar", "topics"] as const,
  topic: (id: number) => ["radar", "topic", id] as const,
  matches: (filters: MatchFilters) => ["radar", "matches", filters] as const,
  match: (id: number) => ["radar", "match", id] as const,
  status: ["radar", "status"] as const,
};

export function useTopicsQuery() {
  return useQuery({
    queryKey: radarKeys.topics,
    queryFn: listTopics,
  });
}

export function useTopicQuery(id: number) {
  return useQuery({
    queryKey: radarKeys.topic(id),
    queryFn: () => getTopic(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useMatchesQuery(filters: MatchFilters) {
  const query = useInfiniteQuery({
    queryKey: radarKeys.matches(filters),
    queryFn: ({ pageParam }) =>
      listMatches({
        limit: PAGE_SIZE,
        offset: pageParam as number,
        topicId: filters.topicId,
        state: filters.state,
      }),
    initialPageParam: 0,
    getNextPageParam: (last: MatchList, all: MatchList[]) => {
      const loaded = all.reduce((s, p) => s + p.items.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });

  const items = (query.data?.pages ?? []).flatMap((p) => p.items);
  const total = query.data?.pages?.[0]?.total ?? 0;
  const hasMore = query.hasNextPage ?? false;

  return {
    items,
    total,
    hasMore,
    isLoading: query.isLoading,
    isSuccess: query.isSuccess,
    isError: query.isError,
    error: query.error,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}

export function useMatchQuery(id: number) {
  return useQuery({
    queryKey: radarKeys.match(id),
    queryFn: () => getMatch(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useRadarStatusQuery() {
  return useQuery({
    queryKey: radarKeys.status,
    queryFn: getStatus,
  });
}
