import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { saveLink } from "@/features/library/api";
import { relativeFromNow, readingTimeLabel } from "@/features/library/time";
import { ArticleBody } from "@/shared/ui/article-body";
import type { MatchFinding } from "../types";

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

export function ReaderSkeleton({ label }: { label: string }) {
  return (
    <div className="max-w-[720px] mx-auto px-4 pt-10" aria-label={label}>
      <div className="skeleton h-10 w-3/4 mb-6" />
      <div className="skeleton h-4 w-1/2 mb-10" />
      <div className="skeleton h-4 w-full mb-3" />
    </div>
  );
}

export function ReaderError({
  notFound,
  notFoundTitle,
  failure,
}: {
  notFound: boolean;
  notFoundTitle: string;
  failure: string;
}) {
  return (
    <div className="max-w-[720px] mx-auto px-4 pt-10 text-center">
      {notFound ? (
        <>
          <h1 className="display-tight text-3xl text-ink mb-3">{notFoundTitle}</h1>
          <Link to="/radar" className="back-link">← Back to radar</Link>
        </>
      ) : (
        <p className="font-body text-muted-foreground">{failure}</p>
      )}
    </div>
  );
}

type Props = {
  finding: MatchFinding;
  /** Where the reader came from, shown above the header. */
  backLink: ReactNode;
  /** Stamps shown above the title. */
  stamps?: ReactNode;
  /** Shown between the header and the article, e.g. the topic marks. */
  aside?: ReactNode;
};

/** The reader body shared by a match and a bare finding: title, byline,
 * the downloaded article (or its summary), and the actions under it. */
export function FindingArticle({ finding: f, backLink, stamps, aside }: Props) {
  const [saving, setSaving] = useState(false);
  const title = f.title ?? f.url;
  const source = f.feedTitle ?? host(f.url);
  const when = f.publishedAt ?? f.discoveredAt;
  const article = f.content;

  async function onSaveToLibrary() {
    setSaving(true);
    try {
      await saveLink(f.url);
      toast.success("Saved to library");
    } catch (e) {
      if (e instanceof ApiError && (e.code === "already_saved" || e.status === 409)) {
        toast.info("Already in library");
      } else {
        toast.error("Could not save");
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <article className="max-w-[720px] mx-auto px-4 pt-8 pb-20">
      {backLink}

      <header className="mb-10">
        {stamps && (
          <div className="flex items-center gap-3 mb-6 flex-wrap">{stamps}</div>
        )}
        <h1 className="display-tight text-4xl md:text-5xl text-ink leading-[1.05] mb-6">
          {title}
        </h1>
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 font-body italic text-base text-muted-foreground">
          {article?.byline && (
            <span>
              by <span className="not-italic text-ink">{article.byline}</span>
            </span>
          )}
          <span>{source}</span>
          <span>{relativeFromNow(when)}</span>
          {article?.readingTimeSeconds != null && (
            <span>{readingTimeLabel(article.readingTimeSeconds)}</span>
          )}
        </div>
      </header>

      {aside}

      <ArticleBody
        html={article?.html}
        text={article?.text}
        fallback={
          f.summary ? (
            <p className="prose-reader font-body text-lg text-ink leading-relaxed whitespace-pre-line">
              {f.summary}
            </p>
          ) : (
            <p className="font-body italic text-muted-foreground">
              No summary captured. Open the original to read.
            </p>
          )
        }
      />

      <div className="mt-16 pt-10 border-t-2 border-ink flex flex-wrap items-center gap-3">
        <a
          href={f.url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex"
        >
          <Button>Open original ↗</Button>
        </a>
        <Button variant="outline" onClick={onSaveToLibrary} disabled={saving}>
          {saving ? "Saving…" : "Save to library"}
        </Button>
      </div>
    </article>
  );
}
