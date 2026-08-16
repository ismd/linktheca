import { relativeFromNow } from "@/features/library/time";
import type { FeedListItem } from "../types";

type Props = {
  feed: FeedListItem;
  isAdmin: boolean;
  onToggle: (subscribed: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
};

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function fmtInterval(seconds: number): string {
  const hours = Math.round(seconds / 3600);
  if (seconds < 3600) return `every ${Math.round(seconds / 60)}m`;
  return hours === 1 ? "every 1h" : `every ${hours}h`;
}

function meta(feed: FeedListItem): string[] {
  const parts = [host(feed.url)];

  if (!feed.isActive) {
    parts.push("paused");
  } else {
    parts.push(fmtInterval(feed.fetchIntervalSeconds));
  }

  if (feed.lastError) {
    parts.push(`⚠ ${feed.lastError}`);
  } else if (feed.lastFetchedAt) {
    parts.push(`fetched ${relativeFromNow(feed.lastFetchedAt)}`);
  } else {
    parts.push("never fetched");
  }

  parts.push(`${feed.findingCount} items`);
  return parts;
}

export function SourceRow({ feed, isAdmin, onToggle, onEdit, onDelete }: Props) {
  const name = feed.title ?? host(feed.url);
  const inputId = `feed-${feed.id}`;

  return (
    <div
      className={`flex items-start justify-between gap-4 py-4 border-b border-rule ${
        feed.isActive ? "" : "opacity-60"
      }`}
    >
      <div className="flex items-start gap-3">
        <input
          id={inputId}
          type="checkbox"
          checked={feed.subscribed}
          onChange={(e) => onToggle(e.target.checked)}
          className="mt-1 h-4 w-4 accent-vermillion"
        />
        <div>
          <label
            htmlFor={inputId}
            className="font-display text-xl text-ink cursor-pointer"
          >
            {name}
          </label>
          <p className="label-sc mt-1 text-muted-foreground">{meta(feed).join(" · ")}</p>
        </div>
      </div>

      {isAdmin && (
        <div className="flex items-center gap-3 shrink-0">
          <button
            type="button"
            onClick={onEdit}
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Edit
          </button>
          <button
            type="button"
            onClick={onDelete}
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
