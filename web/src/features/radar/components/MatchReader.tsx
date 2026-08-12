import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { saveLink } from "@/features/library/api";
import { useMatchQuery } from "../use-radar";
import { useMarkMatchSeen } from "../use-mutations";
import { relativeFromNow } from "@/features/library/time";

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

type Props = { matchId: number };

export function MatchReader({ matchId }: Props) {
  const q = useMatchQuery(matchId);
  const mark = useMarkMatchSeen();
  const marked = useRef(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (q.data && q.data.state === "new" && !marked.current) {
      marked.current = true;
      mark.mutate(matchId);
    }
  }, [q.data, mark, matchId]);

  if (q.isLoading) {
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10" aria-label="Loading match">
        <div className="skeleton h-10 w-3/4 mb-6" />
        <div className="skeleton h-4 w-1/2 mb-10" />
        <div className="skeleton h-4 w-full mb-3" />
      </div>
    );
  }

  if (q.isError) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10 text-center">
        {notFound ? (
          <>
            <h1 className="display-tight text-3xl text-ink mb-3">Match not found</h1>
            <Link to="/radar" className="label-sc text-vermillion">← Back to radar</Link>
          </>
        ) : (
          <p className="font-body text-muted-foreground">Couldn&rsquo;t load this match.</p>
        )}
      </div>
    );
  }

  const m = q.data!;
  const f = m.finding;
  const title = f.title ?? f.url;
  const source = f.feedTitle ?? host(f.url);
  const when = f.publishedAt ?? f.discoveredAt;

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
      <Link
        to={`/radar/topics/${m.topicId}`}
        className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
      >
        ← Back to {m.topicName}
      </Link>

      <header className="mb-10">
        <div className="flex items-center gap-3 mb-6 flex-wrap">
          <Link
            to={`/radar/topics/${m.topicId}`}
            className="stamp text-ink hover:text-vermillion"
          >
            {m.topicName}
          </Link>
        </div>
        <h1 className="display-tight text-4xl md:text-5xl text-ink leading-[1.05] mb-6">
          {title}
        </h1>
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 font-body italic text-base text-muted-foreground">
          <span>{source}</span>
          <span>{relativeFromNow(when)}</span>
        </div>
      </header>

      {f.summary ? (
        <p className="prose-reader font-body text-lg text-ink leading-relaxed whitespace-pre-line">
          {f.summary}
        </p>
      ) : (
        <p className="font-body italic text-muted-foreground">
          No summary captured. Open the original to read.
        </p>
      )}

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
