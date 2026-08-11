import { useSearchParams } from "react-router";
import { PageHeader } from "@/shared/layout/PageHeader";
import { FilterBar } from "@/features/library/components/FilterBar";
import { LibraryGrid } from "@/features/library/components/LibraryGrid";
import type {
  FilterParams,
  LibraryFilterState,
} from "@/features/library/types";

const ALLOWED_STATES: LibraryFilterState[] = ["all", "read", "archived"];

function parseFilters(params: URLSearchParams): FilterParams {
  const state = params.get("state");
  const fav = params.get("favorite");
  return {
    state:
      state && (ALLOWED_STATES as string[]).includes(state)
        ? (state as LibraryFilterState)
        : undefined,
    favorite: fav === "true" ? true : undefined,
  };
}

function nextSearch(current: URLSearchParams, next: FilterParams): URLSearchParams {
  const out = new URLSearchParams(current);
  if (next.state) out.set("state", next.state);
  else out.delete("state");
  if (next.favorite) out.set("favorite", "true");
  else out.delete("favorite");
  return out;
}

export default function LibraryListRoute() {
  const [params, setParams] = useSearchParams();
  const filters = parseFilters(params);

  return (
    <div>
      <PageHeader title="Library" subtitle="Your saved articles" />
      <div className="px-4 lg:px-8 pb-10">
        <FilterBar
          state={filters.state}
          favorite={filters.favorite === true}
          onChange={(next) => setParams(nextSearch(params, next), { replace: true })}
        />
        <div className="pt-8">
          <LibraryGrid filters={filters} />
        </div>
      </div>
    </div>
  );
}
