import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { ApiError } from "@/shared/api/errors";
import { Button } from "@/shared/ui/button";
import { useTopicQuery, useMatchesQuery } from "@/features/radar/use-radar";
import { useUpdateTopic, useDeleteTopic } from "@/features/radar/use-mutations";
import { TopicHeader } from "@/features/radar/components/TopicHeader";
import { StatsLine } from "@/features/radar/components/StatsLine";
import { MatchGrid } from "@/features/radar/components/MatchGrid";
import { EmptyTopicMatches } from "@/features/radar/components/EmptyTopicMatches";
import { EditTopicDialog } from "@/features/radar/components/EditTopicDialog";
import { DeleteTopicConfirm } from "@/features/radar/components/DeleteTopicConfirm";

export default function TopicRoute() {
  const { topicId } = useParams();
  const id = Number(topicId);
  const topic = useTopicQuery(id);
  const matches = useMatchesQuery({ topicId: id });
  const update = useUpdateTopic();
  const del = useDeleteTopic();
  const navigate = useNavigate();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (Number.isNaN(id) || id <= 0) {
    return <div className="p-8 font-body text-muted-foreground">Invalid topic id.</div>;
  }

  if (topic.isLoading) {
    return (
      <div className="px-4 lg:px-8 pt-10" aria-label="Loading topic">
        <div className="skeleton h-12 w-3/4 mb-6" />
        <div className="skeleton h-4 w-1/2 mb-10" />
      </div>
    );
  }

  if (topic.isError) {
    const notFound = topic.error instanceof ApiError && topic.error.status === 404;
    return (
      <div className="px-4 lg:px-8 pt-10">
        {notFound ? (
          <div className="text-center py-20">
            <h1 className="display-tight text-3xl text-ink mb-3">Topic not found</h1>
            <Link to="/radar" className="label-sc text-vermillion">← Back to radar</Link>
          </div>
        ) : (
          <p className="font-body text-muted-foreground">Couldn't load this topic.</p>
        )}
      </div>
    );
  }

  const t = topic.data!;

  function onTogglePause() {
    update.mutate(
      { id: t.id, input: { isActive: !t.isActive } },
      {
        onError: () => toast.error("Could not update — please try again"),
      },
    );
  }

  function onDeleteConfirmed() {
    del.mutate(t.id, {
      onSuccess: () => {
        toast.success("Topic deleted");
        navigate("/radar");
      },
      onError: () => toast.error("Could not delete — please try again"),
    });
  }

  return (
    <div className="px-4 lg:px-8 pt-8 pb-20">
      <Link
        to="/radar"
        className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
      >
        ← Back to radar
      </Link>

      <TopicHeader
        topic={t}
        onEdit={() => setEditOpen(true)}
        onTogglePause={onTogglePause}
        onDelete={() => setDeleteOpen(true)}
        togglePending={update.isPending}
      />
      <StatsLine topic={t} />

      <div className="rule-thick my-8" />

      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-ink">Found entries</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">
          {matches.items.length} shown
        </div>
      </div>

      {matches.isLoading ? (
        <p className="font-body italic text-muted-foreground">Loading…</p>
      ) : matches.items.length === 0 ? (
        <EmptyTopicMatches />
      ) : (
        <>
          <MatchGrid matches={matches.items} />
          {matches.hasMore && (
            <div className="flex justify-center mt-10">
              <Button
                variant="outline"
                onClick={() => matches.fetchNextPage()}
                disabled={matches.isFetchingNextPage}
              >
                {matches.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          )}
        </>
      )}

      <EditTopicDialog topic={t} open={editOpen} onOpenChange={setEditOpen} />
      <DeleteTopicConfirm
        open={deleteOpen}
        topicName={t.name}
        pending={del.isPending}
        onOpenChange={setDeleteOpen}
        onConfirm={onDeleteConfirmed}
      />
    </div>
  );
}
