import { Star } from "lucide-react";
import type { FilterParams, LibraryState } from "../types";

type Props = {
  state: LibraryState | undefined;
  favorite: boolean;
  onChange: (next: FilterParams) => void;
};

const STATES: { label: string; value: LibraryState | undefined }[] = [
  { label: "All", value: undefined },
  { label: "Unread", value: "unread" },
  { label: "Read", value: "read" },
  { label: "Archived", value: "archived" },
];

export function FilterBar({ state, favorite, onChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3 py-4 border-b border-rule">
      <div className="flex flex-wrap gap-1" role="group" aria-label="State filter">
        {STATES.map((opt) => {
          const active = state === opt.value;
          return (
            <button
              key={opt.label}
              type="button"
              aria-pressed={active}
              onClick={() =>
                onChange({ state: opt.value, favorite: favorite || undefined })
              }
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

      <div className="ml-auto">
        <button
          type="button"
          onClick={() => onChange({ state, favorite: !favorite })}
          className={
            favorite
              ? "px-3 py-1.5 label-sc bg-ochre text-paper inline-flex items-center gap-2"
              : "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2 inline-flex items-center gap-2"
          }
        >
          <Star className="h-3.5 w-3.5" strokeWidth={1.5} aria-hidden="true" />
          Favorites only
        </button>
      </div>
    </div>
  );
}
