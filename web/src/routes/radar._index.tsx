import { Link, useSearchParams } from "react-router";
import { ApiError } from "@/shared/api/errors";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import {
  useTopicsQuery,
  useMatchesQuery,
  useRadarStatusQuery,
} from "@/features/radar/use-radar";
import { useNewTopicStore } from "@/features/radar/use-new-topic-store";
import { fmtSweep } from "@/features/radar/time";
import { InboxFilterBar } from "@/features/radar/components/InboxFilterBar";
import { MatchGrid } from "@/features/radar/components/MatchGrid";
import { EmptyInbox } from "@/features/radar/components/EmptyInbox";
import { EmptyTopicList } from "@/features/radar/components/EmptyTopicList";
import { EmptyTopicMatches } from "@/features/radar/components/EmptyTopicMatches";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
import type { InboxFilters } from "@/features/radar/types";

function parseFilters(params: URLSearchParams): InboxFilters {
  const topic = Number(params.get("topic"));
  return {
    state: params.get("state") === "all" ? "all" : "new",
    topicId: Number.isInteger(topic) && topic > 0 ? topic : undefined,
  };
}

function nextSearch(
  current: URLSearchParams,
  next: InboxFilters,
): URLSearchParams {
  const out = new URLSearchParams(current);
  if (next.state === "all") out.set("state", "all");
  else out.delete("state");
  if (next.topicId !== undefined) out.set("topic", String(next.topicId));
  else out.delete("topic");
  return out;
}

export default function RadarInboxRoute() {
  const [params, setParams] = useSearchParams();
  const filters = parseFilters(params);
  const topics = useTopicsQuery();
  const status = useRadarStatusQuery();
  const matches = useMatchesQuery({
    topicId: filters.topicId,
    state: filters.state === "new" ? "new" : undefined,
  });
  const openNewTopic = useNewTopicStore((s) => s.open);

  if (
    topics.error instanceof ApiError &&
    topics.error.code === "radar_disabled"
  ) {
    return <RadarDisabled />;
  }

  const noTopics = topics.isSuccess && topics.data.length === 0;

  return (
    <div>
      <PageHeader
        title="Radar"
        subtitle={fmtSweep(status.data?.lastSweepAt ?? null)}
        actions={
          <>
            <Link
              to="/radar/topics"
              className="label-sc text-muted-foreground hover:text-vermillion"
            >
              Topics →
            </Link>
            <Link
              to="/radar/sources"
              className="label-sc text-muted-foreground hover:text-vermillion"
            >
              Sources →
            </Link>
          </>
        }
      />
      <div className="px-4 lg:px-8 pb-10">
        {noTopics ? (
          <div className="pt-8">
            <EmptyTopicList onAdd={openNewTopic} />
          </div>
        ) : (
          <>
            <InboxFilterBar
              state={filters.state}
              topicId={filters.topicId}
              topics={topics.data ?? []}
              onChange={(next) =>
                setParams(nextSearch(params, next), { replace: true })
              }
            />
            <div className="pt-8">
              {matches.isLoading ? (
                <p className="font-body italic text-muted-foreground">Loading…</p>
              ) : matches.items.length === 0 ? (
                filters.state === "new" ? (
                  <EmptyInbox />
                ) : (
                  <EmptyTopicMatches />
                )
              ) : (
                <>
                  <MatchGrid matches={matches.items} showTopic />
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
            </div>
          </>
        )}
      </div>
    </div>
  );
}
