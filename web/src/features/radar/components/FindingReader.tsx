import { Link } from "react-router";
import { ApiError } from "@/shared/api/errors";
import { useFindingQuery } from "../use-radar";
import { FindingArticle, ReaderError, ReaderSkeleton } from "./FindingArticle";

type Props = { findingId: number };

/** Reads a finding that is not (necessarily) a match — what a topic preview
 * row opens, so the article can be checked before the topic is saved. */
export function FindingReader({ findingId }: Props) {
  const q = useFindingQuery(findingId);

  if (q.isLoading) {
    return <ReaderSkeleton label="Loading article" />;
  }

  if (q.isError) {
    return (
      <ReaderError
        notFound={q.error instanceof ApiError && q.error.status === 404}
        notFoundTitle="Article not found"
        failure="Couldn’t load this article."
      />
    );
  }

  return (
    <FindingArticle
      finding={q.data!}
      backLink={
        <Link to="/radar" className="back-link inline-block mb-10">
          ← Back to radar
        </Link>
      }
    />
  );
}
