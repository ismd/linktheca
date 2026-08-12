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

const CHIP_ACTIVE = "px-3 py-1.5 label-sc bg-ink text-paper inline-flex items-center gap-2";
const CHIP_IDLE = "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2 inline-flex items-center gap-2";

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

export function InboxFilterBar({ state, topicId, topics, onChange }: Props) {
  const chips = visibleTopics(topics, topicId);
  return (
    <div className="py-4 border-b border-rule">
      <div className="flex flex-wrap gap-1" role="group" aria-label="State filter">
        {STATES.map((opt) => {
          const active = state === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              aria-pressed={active}
              onClick={() => onChange({ state: opt.value, topicId })}
              className={
                active
                  ? "px-3 py-1.5 label-sc bg-ink text-paper cursor-auto"
                  : "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2"
              }
            >
              {opt.label}
            </button>
          );
        })}
      </div>

      <div className="flex flex-wrap gap-1 mt-3" role="group" aria-label="Topic filter">
        <button
          type="button"
          aria-pressed={topicId === undefined}
          onClick={() => onChange({ state, topicId: undefined })}
          className={topicId === undefined ? CHIP_ACTIVE : CHIP_IDLE}
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
              aria-pressed={active}
              onClick={() => onChange({ state, topicId: active ? undefined : t.id })}
              className={active ? CHIP_ACTIVE : CHIP_IDLE}
            >
              {t.name}
              {count > 0 && (
                <span className={active ? "text-paper" : "text-vermillion"}>
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
