import { useEffect, useRef } from "react";
import { Link } from "react-router";
import { ApiError } from "@/shared/api/errors";
import { useMatchQuery } from "../use-radar";
import { useMarkMatchSeen } from "../use-mutations";
import { FindingArticle, ReaderError, ReaderSkeleton } from "./FindingArticle";

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
    />
  );
}
