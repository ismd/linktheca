import { useState } from "react";
import { toast } from "sonner";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useFeedsQuery, useRadarStatusQuery } from "@/features/radar/use-radar";
import {
  useToggleSubscription,
  useDeleteFeed,
} from "@/features/radar/use-mutations";
import { SourceRow } from "@/features/radar/components/SourceRow";
import { AddFeedDialog } from "@/features/radar/components/AddFeedDialog";
import { EditFeedDialog } from "@/features/radar/components/EditFeedDialog";
import { DeleteFeedConfirm } from "@/features/radar/components/DeleteFeedConfirm";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
import type { FeedListItem } from "@/features/radar/types";

export default function SourcesRoute() {
  const feeds = useFeedsQuery();
  const status = useRadarStatusQuery();
  const toggle = useToggleSubscription();
  const remove = useDeleteFeed();

  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];
  const mine = items.filter((f) => f.isOwn);
  const catalog = items.filter((f) => !f.isOwn);
  const subscribedCount = items.filter((f) => f.subscribed).length;
  const quota = status.data?.maxUserFeeds;

  return (
    <div>
      <PageHeader
        title="Sources"
        subtitle={
          items.length
            ? `${items.length} feeds · ${subscribedCount} subscribed · changes apply from the next sweep`
            : "Feeds this instance watches"
        }
        actions={<Button onClick={() => setAddOpen(true)}>Add source</Button>}
      />
      <div className="px-4 lg:px-8 pb-10">
        <SectionHeader
          label="My sources"
          count={quota ? `${mine.length} / ${quota}` : `${mine.length}`}
        />
        {mine.length === 0 ? (
          <p className="font-body text-muted-foreground pb-6">
            Add your own RSS or Atom feed — only you will see it.
          </p>
        ) : (
          mine.map((feed) => (
            <SourceRow
              key={feed.id}
              feed={feed}
              canManage
              onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
              onEdit={() => setEditing(feed)}
              onDelete={() => setDeleting(feed)}
            />
          ))
        )}

        <SectionHeader label="Catalog" count={`${catalog.length} feeds`} />
        {catalog.length === 0 ? (
          <p className="font-body text-muted-foreground pb-6">
            No shared sources yet. Ask the instance admin to add feeds.
          </p>
        ) : (
          catalog.map((feed) => (
            <SourceRow
              key={feed.id}
              feed={feed}
              canManage={false}
              onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
            />
          ))
        )}
      </div>

      <AddFeedDialog open={addOpen} scope="personal" onOpenChange={setAddOpen} />
      <EditFeedDialog
        feed={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />
      <DeleteFeedConfirm
        feed={deleting}
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={async () => {
          if (!deleting) return;
          try {
            await remove.mutateAsync(deleting.id);
            toast.success("Source deleted");
          } catch {
            toast.error("Could not delete the source");
          } finally {
            setDeleting(null);
          }
        }}
      />
    </div>
  );
}

// SectionHeader repeats the divider used on the topics list.
function SectionHeader({ label, count }: { label: string; count: string }) {
  return (
    <div className="flex items-center gap-4 pt-8 pb-4">
      <div className="label-sc-lg text-ink">{label}</div>
      <div className="flex-1 rule-dotted" />
      <div className="label-sc text-muted-foreground">{count}</div>
    </div>
  );
}
