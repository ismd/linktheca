import { useState } from "react";
import { toast } from "sonner";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import {
  useGlobalFeedsQuery,
  useDeleteGlobalFeed,
} from "@/features/admin/use-admin-feeds";
import { SourceRow } from "@/features/radar/components/SourceRow";
import { AddFeedDialog } from "@/features/radar/components/AddFeedDialog";
import { EditFeedDialog } from "@/features/radar/components/EditFeedDialog";
import { DeleteFeedConfirm } from "@/features/radar/components/DeleteFeedConfirm";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
import type { FeedListItem } from "@/features/radar/types";

export default function AdminSourcesRoute() {
  const feeds = useGlobalFeedsQuery();
  const remove = useDeleteGlobalFeed();

  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];

  return (
    <div>
      <PageHeader
        title="Global sources"
        subtitle={
          items.length
            ? `${items.length} feeds · every account can subscribe to them`
            : "Feeds every account can subscribe to"
        }
        actions={<Button onClick={() => setAddOpen(true)}>Add feed</Button>}
      />
      <div className="px-4 lg:px-8 pb-10">
        {feeds.isSuccess && items.length === 0 && (
          <p className="font-body text-muted-foreground pt-8">
            Add the first shared feed to start watching.
          </p>
        )}
        {items.map((feed) => (
          <SourceRow
            key={feed.id}
            feed={feed}
            canManage
            onEdit={() => setEditing(feed)}
            onDelete={() => setDeleting(feed)}
          />
        ))}
      </div>

      <AddFeedDialog open={addOpen} scope="global" onOpenChange={setAddOpen} />
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
            toast.success("Feed deleted");
          } catch {
            toast.error("Could not delete the feed");
          } finally {
            setDeleting(null);
          }
        }}
      />
    </div>
  );
}
