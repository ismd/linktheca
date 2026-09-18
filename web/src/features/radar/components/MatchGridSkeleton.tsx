// Mirrors MatchCard's frame and MatchGrid's columns so the first load reserves
// the space the real cards will take, instead of collapsing to a line of text.
function MatchSkeletonCard() {
  return (
    <article
      data-testid="match-skeleton-card"
      className="flex flex-col h-full p-5 border border-rule"
    >
      <div className="flex items-center gap-x-3 mb-3">
        <div className="skeleton h-3 w-10" />
        <div className="skeleton h-3 w-24" />
      </div>
      <div className="skeleton h-6 w-5/6 mb-2" />
      <div className="skeleton h-6 w-2/3 mb-4" />
      <div className="skeleton h-4 w-full mb-2" />
      <div className="skeleton h-4 w-4/5" />
    </article>
  );
}

export function MatchGridSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div
      role="status"
      aria-label="Loading matches"
      className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6"
    >
      {Array.from({ length: count }).map((_, i) => (
        <MatchSkeletonCard key={i} />
      ))}
    </div>
  );
}
