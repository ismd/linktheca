import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { listLibrary, getLibraryDetail, getLibraryItem } from "./api";
import {
  PAGE_SIZE,
  type FilterParams,
  type LibraryFilterState,
  type LibraryState,
  type ListPage,
} from "./types";

export const libraryKeys = {
  all: ["library"] as const,
  list: (filters: FilterParams) => ["library", "list", filters] as const,
  item: (id: number) => ["library", "item", id] as const,
  detail: (id: number) => ["library", "detail", id] as const,
};

export function useLibraryQuery(filters: FilterParams) {
  const query = useInfiniteQuery({
    queryKey: libraryKeys.list(filters),
    queryFn: ({ pageParam }) =>
      listLibrary({
        limit: PAGE_SIZE,
        offset: pageParam as number,
        state: prepareState(filters.state),
        favorite: filters.favorite,
      }),
    initialPageParam: 0,
    getNextPageParam: (last: ListPage, all: ListPage[]) => {
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

export function useLibraryItemQuery(id: number) {
  return useQuery({
    queryKey: libraryKeys.item(id),
    queryFn: () => getLibraryItem(id),
  });
}

export function useLibraryItemDetailQuery(id: number) {
  return useQuery({
    queryKey: libraryKeys.detail(id),
    queryFn: () => getLibraryDetail(id),
  });
}

function prepareState(state?: LibraryFilterState): LibraryState | undefined {
  return state === "all" ? undefined : (state ?? "unread");
}
