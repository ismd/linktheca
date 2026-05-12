export function SkeletonCard() {
  return (
    <article className="flex flex-col h-full" data-testid="library-skeleton-card">
      <div className="skeleton aspect-[16/10] w-full mb-5" />
      <div className="skeleton h-4 w-1/3 mb-3" />
      <div className="skeleton h-7 w-5/6 mb-4" />
      <div className="skeleton h-4 w-full mb-1" />
      <div className="skeleton h-4 w-2/3" />
    </article>
  );
}
