import { Link } from "react-router";
import { useState } from "react";
import { Star } from "lucide-react";
import { gradientClassFor, previewImageUrl } from "../image";
import { relativeFromNow, readingTimeLabel } from "../time";
import { LibraryCardMenu } from "./LibraryCardMenu";
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
  const [imageFailed, setImageFailed] = useState(false);
  const showImage = Boolean(item.image) && !imageFailed;
  const href = `/library/${item.id}`;
  return (
    <article className="feed-card group flex flex-col h-full">
      <Link to={href} tabIndex={-1} aria-hidden="true" className="block mb-5">
        <div
          className={`${gradientClassFor(item.id)} relative overflow-hidden`}
          style={{ aspectRatio: "16 / 10" }}
        >
          {showImage && (
            <>
              <img
                src={previewImageUrl(item.image!)}
                alt=""
                loading="lazy"
                decoding="async"
                onError={() => setImageFailed(true)}
                className="absolute inset-0 h-full w-full object-cover"
              />
              {/* The gradients are dark by design; a real photo may not be, so
                  the reading-time label needs its own footing. */}
              <div className="absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-black/70 to-transparent" />
            </>
          )}

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
      </Link>

      <div className="flex items-center gap-2 mb-3">
        <span className="label-sc text-muted-foreground truncate">
          {host(item.url)}
        </span>
        <span className="label-sc text-muted-foreground shrink-0">·</span>
        <span className="label-sc text-muted-foreground shrink-0">
          {relativeFromNow(item.savedAt)}
        </span>
        <div className="ml-auto shrink-0 transition-opacity [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:group-focus-within:opacity-100 [@media(hover:hover)]:has-[[data-state=open]]:opacity-100">
          <LibraryCardMenu item={item} />
        </div>
      </div>

      <Link to={href} className="block">
        <h2 className="card-title display-tight text-2xl text-ink leading-[1.1] mb-3 break-words">
          {title}
        </h2>

        {item.excerpt && (
          <p className="font-body text-ink-3 leading-[1.55] line-clamp-3">
            {item.excerpt}
          </p>
        )}
      </Link>
    </article>
  );
}
