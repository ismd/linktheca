import type { InboxFilters, InboxState, TopicWithStats } from "../types";

type Props = {
  state: InboxState;
  topicId: number | undefined;
  topics: TopicWithStats[];
  onChange: (next: InboxFilters) => void;
};

const STATES: { label: string; value: InboxState }[] = [
  { label: "New", value: "new" },
  { label: "All", value: "all" },
];

// Active topics are always offered. A paused topic only earns a chip while it
// still has unread matches — or while it is the current filter, so a selection
// arriving from the URL is never invisible.
export function visibleTopics(
  topics: TopicWithStats[],
  selectedId: number | undefined,
): TopicWithStats[] {
  return topics.filter(
    (t) => t.isActive || t.stats.newCount > 0 || t.id === selectedId,
  );
}

// Both filters live on one line: read/unread is a two-way switch, so it is a
// joined segmented control; topic is a scope, so it is a row of separate chips.
// The rule between them keeps the two from reading as one confused strip.
export function InboxFilterBar({ state, topicId, topics, onChange }: Props) {
  const chips = visibleTopics(topics, topicId);
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-3 py-4 border-b border-rule">
      <div className="segmented" role="group" aria-label="State filter">
        {STATES.map((opt) => (
          <button
            key={opt.value}
            type="button"
            aria-pressed={state === opt.value}
            onClick={() => onChange({ state: opt.value, topicId })}
          >
            {opt.label}
          </button>
        ))}
      </div>

      <span aria-hidden="true" className="hidden sm:block h-5 w-px bg-rule mx-1" />

      <div className="flex flex-wrap gap-1.5" role="group" aria-label="Topic filter">
        <button
          type="button"
          className="filter-chip"
          aria-pressed={topicId === undefined}
          onClick={() => onChange({ state, topicId: undefined })}
        >
          All topics
        </button>
        {chips.map((t) => {
          const active = topicId === t.id;
          const count = t.stats.newCount;
          return (
            <button
              key={t.id}
              type="button"
              className="filter-chip"
              aria-pressed={active}
              onClick={() => onChange({ state, topicId: active ? undefined : t.id })}
            >
              {t.name}
              {count > 0 && (
                <span className={active ? "text-paper/70" : "text-vermillion"}>
                  {count}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
