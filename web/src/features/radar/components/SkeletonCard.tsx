export function SkeletonCard() {
  return (
    <div className="topic-card block p-6 animate-pulse">
      <div className="flex items-start justify-between mb-3">
        <div className="skeleton h-3 w-20" />
        <div className="skeleton h-3 w-12" />
      </div>
      <div className="skeleton h-7 w-2/3 mb-3" />
      <div className="skeleton h-4 w-full mb-2" />
      <div className="skeleton h-4 w-5/6 mb-5" />
      <div className="rule-dotted mb-4" />
      <div className="flex items-center justify-between">
        <div className="skeleton h-3 w-1/2" />
        <div className="skeleton h-3 w-1/4" />
      </div>
    </div>
  );
}
