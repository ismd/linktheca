import { PageHeader } from "@/shared/layout/PageHeader";
import { ApiError } from "@/shared/api/errors";
import { useAuthStore } from "@/features/auth/store";
import { useFeedsQuery } from "@/features/radar/use-radar";
import { useToggleSubscription } from "@/features/radar/use-mutations";
import { SourceRow } from "@/features/radar/components/SourceRow";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";

export default function SourcesRoute() {
  const feeds = useFeedsQuery();
  const toggle = useToggleSubscription();
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);

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
            onEdit={() => {}}
            onDelete={() => {}}
          />
        ))}
      </div>
    </div>
  );
}
