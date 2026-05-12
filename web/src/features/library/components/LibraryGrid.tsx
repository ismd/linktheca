import { useLibraryQuery } from "../use-library";
import type { FilterParams } from "../types";
import { LibraryCard } from "./LibraryCard";
import { SkeletonCard } from "./SkeletonCard";
import { EmptyState } from "./EmptyState";
import { ErrorPanel } from "./ErrorPanel";
import { Button } from "@/shared/ui/button";

type Props = {
  filters: FilterParams;
};

export function LibraryGrid({ filters }: Props) {
  const q = useLibraryQuery(filters);
  const filtered = filters.state !== undefined || filters.favorite === true;

  if (q.isLoading) {
    return (
      <div
        role="status"
        aria-label="Loading library"
        className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8"
      >
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  if (q.isError) {
    return (
      <ErrorPanel
        message={
          q.error instanceof Error ? q.error.message : "Failed to load library"
        }
        onRetry={() => q.refetch()}
      />
    );
  }

  if (q.items.length === 0) {
    return <EmptyState filtered={filtered} />;
  }

  return (
    <div className="flex flex-col gap-10">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
        {q.items.map((item) => (
          <LibraryCard key={item.id} item={item} />
        ))}
      </div>

      {q.hasMore && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={() => q.fetchNextPage()}
            disabled={q.isFetchingNextPage}
          >
            {q.isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
