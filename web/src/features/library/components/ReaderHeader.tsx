import { faviconUrl } from "../image";
import { relativeFromNow, readingTimeLabel } from "../time";
import type { LibraryItemDetail } from "../types";

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

type Props = {
  detail: LibraryItemDetail;
};

export function ReaderHeader({ detail }: Props) {
  const title = detail.content.title ?? detail.title ?? detail.url;
  return (
    <header className="mb-10">
      <h1 className="display-tight text-4xl md:text-5xl text-ink leading-[1.05] mb-6">
        {title}
      </h1>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 font-body italic text-base text-muted-foreground">
        {detail.content.byline && (
          <span>
            by{" "}
            <span className="not-italic text-ink">{detail.content.byline}</span>
          </span>
        )}
        <span className="inline-flex items-center gap-1.5">
          {detail.content.favicon && (
            <img
              src={faviconUrl(detail.content.favicon)}
              alt=""
              width={16}
              height={16}
              className="h-4 w-4 shrink-0 rounded-[2px]"
            />
          )}
          {host(detail.url)}
        </span>
        <span>{relativeFromNow(detail.savedAt)}</span>
        <span>{readingTimeLabel(detail.content.readingTimeSeconds ?? detail.readingTimeSeconds)}</span>
      </div>
    </header>
  );
}
