import { useNavigate, useParams, Link } from "react-router";
import { useCallback, useState } from "react";
import { useLibraryItemDetailQuery } from "@/features/library/use-library";
import { useUpdateItem } from "@/features/library/use-mutations";
import { ReadingProgress } from "@/features/library/components/ReadingProgress";
import { ReaderHeader } from "@/features/library/components/ReaderHeader";
import { ReaderActions } from "@/features/library/components/ReaderActions";
import { ErrorPanel } from "@/features/library/components/ErrorPanel";
import { useMarkReadOnScroll } from "@/features/library/components/useMarkReadOnScroll";
import { ReaderHero } from "@/features/library/components/ReaderHero";
import { ApiError } from "@/shared/api/errors";

function LoadingState() {
  return (
    <div className="max-w-[720px] mx-auto px-4 pt-10" aria-label="Loading article">
      <div className="skeleton h-10 w-3/4 mb-6" />
      <div className="skeleton h-4 w-1/2 mb-10" />
      <div className="skeleton h-[300px] w-full mb-10" />
      <div className="skeleton h-4 w-full mb-3" />
      <div className="skeleton h-4 w-5/6 mb-3" />
      <div className="skeleton h-4 w-4/6" />
    </div>
  );
}

export default function LibraryItemRoute() {
  const { id } = useParams();
  const itemId = Number(id);
  const navigate = useNavigate();
  const detail = useLibraryItemDetailQuery(itemId);
  const update = useUpdateItem();
  const [handToggledId, setHandToggledId] = useState<number | null>(null);
  const readStateSetByHand = handToggledId === itemId;

  const onReach = useCallback(() => {
    if (!detail.data) return;
    if (detail.data.state !== "unread") return;
    update.mutate({ id: itemId, input: { state: "read" } });
  }, [detail.data, itemId, update]);

  useMarkReadOnScroll({
    enabled: detail.data?.state === "unread" && !readStateSetByHand,
    onReach,
  });

  if (Number.isNaN(itemId)) {
    return (
      <div className="p-8">
        <p className="font-body text-ink-3">Invalid article id.</p>
      </div>
    );
  }

  if (detail.isLoading) return <LoadingState />;

  if (detail.isError) {
    const notFound =
      detail.error instanceof ApiError && detail.error.status === 404;
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10">
        {notFound ? (
          <div className="text-center py-20">
            <h1 className="display-tight text-4xl text-ink mb-3">
              Article not found
            </h1>
            <Link to="/library" className="label-sc text-vermillion">
              ← Back to library
            </Link>
          </div>
        ) : (
          <ErrorPanel
            message="Couldn't load this article"
            onRetry={() => detail.refetch()}
          />
        )}
      </div>
    );
  }

  const d = detail.data!;
  const fetchFailed = Boolean(d.content.fetchError);

  return (
    <>
      <ReadingProgress />
      <article className="max-w-[720px] mx-auto px-4 pt-8 pb-20">
        <Link
          to="/library"
          className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
        >
          ← Back to library
        </Link>

        <ReaderHeader detail={d} />

        <ReaderHero id={d.id} image={d.content.image ?? d.image} />

        {fetchFailed ? (
          <div className="border border-rule bg-paper-2 px-5 py-6">
            <p className="label-sc text-muted-foreground mb-2">Extraction failed</p>
            <p className="font-body text-ink-3 mb-4">
              We couldn&apos;t extract content from this page. Open the original to read it.
            </p>
            <p className="font-mono text-xs text-muted-foreground">
              {d.content.fetchError}
            </p>
          </div>
        ) : d.content.html ? (
          <div
            className="prose-reader drop-cap"
            dangerouslySetInnerHTML={{ __html: d.content.html }}
          />
        ) : d.content.text ? (
          <div className="prose-reader drop-cap whitespace-pre-wrap">
            {d.content.text}
          </div>
        ) : (
          <p className="font-body italic text-muted-foreground">
            No readable content for this URL.
          </p>
        )}

        <ReaderActions
          item={d}
          onReadStateToggled={() => setHandToggledId(itemId)}
          onDeleted={() => navigate("/library")}
        />
      </article>
    </>
  );
}
