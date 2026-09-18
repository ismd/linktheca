import { Star } from "lucide-react";
import type { FilterParams, LibraryFilterState } from "../types";

type Props = {
  state: LibraryFilterState | undefined;
  favorite: boolean;
  onChange: (next: FilterParams) => void;
};

const STATES: { label: string; value: LibraryFilterState | undefined }[] = [
  { label: "All", value: "all" },
  { label: "Unread", value: undefined },
  { label: "Read", value: "read" },
  { label: "Archived", value: "archived" },
];

export function FilterBar({ state, favorite, onChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3 py-4 border-b border-rule">
      <div className="segmented" role="group" aria-label="State filter">
        {STATES.map((opt) => (
          <button
            key={opt.label}
            type="button"
            aria-pressed={state === opt.value}
            onClick={() =>
              onChange({ state: opt.value, favorite: favorite || undefined })
            }
          >
            {opt.label}
          </button>
        ))}
      </div>

      <div className="ml-auto">
        <button
          type="button"
          className="filter-chip filter-chip-fav"
          aria-pressed={favorite}
          onClick={() => onChange({ state, favorite: !favorite })}
        >
          <Star className="h-3.5 w-3.5" strokeWidth={1.5} aria-hidden="true" />
          Favorites only
        </button>
      </div>
    </div>
  );
}
