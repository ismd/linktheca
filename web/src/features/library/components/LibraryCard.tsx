import { Link } from "react-router";
import { Star } from "lucide-react";
import { gradientClassFor } from "../image";
import { relativeFromNow, readingTimeLabel } from "../time";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
};

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function LibraryCard({ item }: Props) {
  const title = item.title ?? item.url;
  const isRead = item.state === "read";
  return (
    <Link
      to={`/library/${item.id}`}
      className="feed-card group block"
      aria-label={title}
    >
      <article className="flex flex-col h-full">
        <div
          className={`${gradientClassFor(item.id)} relative overflow-hidden mb-5`}
          style={{ aspectRatio: "16 / 10" }}
        >
          <div className="absolute top-3 left-3 flex gap-2">
            <span
              className={`stamp bg-paper/95 stamp-flat ${
                isRead ? "text-sage" : "text-vermillion"
              }`}
            >
              {isRead ? "✓ read" : "✦ saved"}
            </span>
            {item.isFavorite && (
              <span
                aria-label="favorite"
                className="stamp bg-paper/95 stamp-flat text-ochre"
              >
                <Star
                  className="inline h-3 w-3 mr-1"
                  strokeWidth={2}
                  aria-hidden="true"
                />
                favorite
              </span>
            )}
          </div>
          <div className="absolute bottom-3 right-3 label-sc text-paper/80">
            {readingTimeLabel(item.readingTimeSeconds)}
          </div>
        </div>

        <div className="flex items-center gap-2 mb-3 flex-wrap">
          <span className="label-sc text-muted-foreground">{host(item.url)}</span>
          <span className="label-sc text-muted-foreground">·</span>
          <span className="label-sc text-muted-foreground">
            {relativeFromNow(item.savedAt)}
          </span>
        </div>

        <h2 className="card-title display-tight text-2xl text-ink leading-[1.1] mb-3">
          {title}
        </h2>

        {item.excerpt && (
          <p className="font-body text-ink-3 leading-[1.55] line-clamp-3">
            {item.excerpt}
          </p>
        )}
      </article>
    </Link>
  );
}
