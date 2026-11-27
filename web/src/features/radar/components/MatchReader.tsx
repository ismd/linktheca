import { useEffect, useRef } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { ApiError } from "@/shared/api/errors";
import { Button } from "@/shared/ui/button";
import { useMatchQuery } from "../use-radar";
import { useMarkMatchSeen, useSetTopicMark } from "../use-mutations";
import type { MatchView, Verdict } from "../types";
import { FindingArticle, ReaderError, ReaderSkeleton } from "./FindingArticle";

/** Asks whether the match is about its topic; the answer teaches the topic. */
function MatchVerdict({ match: m }: { match: MatchView }) {
  const setMark = useSetTopicMark();
  // Show the answer right away rather than after the match is read again.
  const verdict = setMark.isPending ? setMark.variables.verdict : m.verdict;

  function mark(v: Verdict | null) {
    setMark.mutate(
      { topicId: m.topicId, findingId: m.finding.id, verdict: v },
      { onError: () => toast.error("Could not save the mark — please try again") },
    );
  }

  const undo = (
    <Button variant="ghost" size="sm" onClick={() => mark(null)} disabled={setMark.isPending}>
      Undo
    </Button>
  );

  return (
    <div
      className="mb-10 flex flex-wrap items-center gap-x-4 gap-y-2 border-y border-rule py-3"
      aria-live="polite"
    >
      {verdict === null ? (
        <>
          <span className="label-sc text-ink-3">Is this about {m.topicName}?</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => mark("relevant")} disabled={setMark.isPending}>
              ✓ About it
            </Button>
            <Button variant="outline" size="sm" onClick={() => mark("irrelevant")} disabled={setMark.isPending}>
              ✗ Not about it
            </Button>
          </div>
        </>
      ) : verdict === "relevant" ? (
        <>
          <span className="label-sc text-ink">✓ Marked as about {m.topicName}</span>
          {undo}
        </>
      ) : (
        <>
          <span className="label-sc text-vermillion">
            ✗ Hidden from {m.topicName}; articles that score as low go with it
          </span>
          {undo}
        </>
      )}
    </div>
  );
}

type Props = { matchId: number };

export function MatchReader({ matchId }: Props) {
  const q = useMatchQuery(matchId);
  const mark = useMarkMatchSeen();
  const marked = useRef(false);

  useEffect(() => {
    if (q.data && q.data.state === "new" && !marked.current) {
      marked.current = true;
      mark.mutate(matchId);
    }
  }, [q.data, mark, matchId]);

  if (q.isLoading) {
    return <ReaderSkeleton label="Loading match" />;
  }

  if (q.isError) {
    return (
      <ReaderError
        notFound={q.error instanceof ApiError && q.error.status === 404}
        notFoundTitle="Match not found"
        failure="Couldn’t load this match."
      />
    );
  }

  const m = q.data!;

  return (
    <FindingArticle
      finding={m.finding}
      backLink={
        <Link
          to={`/radar/topics/${m.topicId}`}
          className="back-link inline-block mb-10"
        >
          ← Back to {m.topicName}
        </Link>
      }
      stamps={
        <Link
          to={`/radar/topics/${m.topicId}`}
          className="stamp text-ink hover:text-vermillion"
        >
          {m.topicName}
        </Link>
      }
      aside={<MatchVerdict match={m} />}
    />
  );
}
