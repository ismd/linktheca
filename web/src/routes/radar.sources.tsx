import { useState } from "react";
import { toast } from "sonner";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useAuthStore } from "@/features/auth/store";
import { useFeedsQuery } from "@/features/radar/use-radar";
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
  const toggle = useToggleSubscription();
  const remove = useDeleteFeed();
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);

  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];
  const subscribedCount = items.filter((f) => f.subscribed).length;

  return (
    <div>
      <PageHeader
        title="Sources"
        subtitle={
          items.length
            ? `${items.length} feeds · ${subscribedCount} subscribed · changes apply from the next sweep`
            : "Feeds this instance watches"
        }
        actions={
          isAdmin ? <Button onClick={() => setAddOpen(true)}>Add feed</Button> : undefined
        }
      />
      <div className="px-4 lg:px-8 pb-10">
        {feeds.isSuccess && items.length === 0 && (
          <p className="font-body text-muted-foreground pt-8">
            {isAdmin
              ? "No sources yet. Add the first feed to start watching."
              : "No sources yet. Ask the instance admin to add feeds."}
          </p>
        )}
        {items.map((feed) => (
          <SourceRow
            key={feed.id}
            feed={feed}
            isAdmin={isAdmin}
            onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
            onEdit={() => setEditing(feed)}
            onDelete={() => setDeleting(feed)}
          />
        ))}
      </div>

      <AddFeedDialog open={addOpen} onOpenChange={setAddOpen} />
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
