import { ApiError } from "@/shared/api/errors";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { useTopicsQuery, useRadarStatusQuery } from "@/features/radar/use-radar";
import { useNewTopicStore } from "@/features/radar/use-new-topic-store";
import { fmtSweep } from "@/features/radar/time";
import { TopicGrid } from "@/features/radar/components/TopicGrid";
import { EmptyTopicList } from "@/features/radar/components/EmptyTopicList";
import { SkeletonCard } from "@/features/radar/components/SkeletonCard";

function LoadingGrid() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
      <SkeletonCard />
      <SkeletonCard />
      <SkeletonCard />
      <SkeletonCard />
    </div>
  );
}

function RadarDisabled() {
  return (
    <div className="text-center py-20">
      <p className="display-tight text-3xl text-ink mb-3">Radar is disabled</p>
      <p className="font-body italic text-muted-foreground">
        This Linktheca instance was started with Radar turned off.
      </p>
    </div>
  );
}

export default function RadarListRoute() {
  const topics = useTopicsQuery();
  const status = useRadarStatusQuery();
  const openNewTopic = useNewTopicStore((s) => s.open);

  if (
    topics.error instanceof ApiError &&
    topics.error.code === "radar_disabled"
  ) {
    return <RadarDisabled />;
  }

  return (
    <div>
      <PageHeader
        title="Radar"
        subtitle={fmtSweep(status.data?.lastSweepAt ?? null)}
      />
      <div className="px-4 lg:px-8 pb-10">
        <div className="hidden md:flex justify-end mb-6">
          <Button onClick={openNewTopic}>+ New topic</Button>
        </div>
        <div className="md:hidden mb-8">
          <Button className="w-full" onClick={openNewTopic}>+ New topic</Button>
        </div>

        {topics.isLoading && <LoadingGrid />}

        {topics.isSuccess && topics.data.length === 0 && (
          <EmptyTopicList onAdd={openNewTopic} />
        )}

        {topics.isSuccess && topics.data.length > 0 && (
          <>
            <ActiveSection topics={topics.data.filter((t) => t.isActive)} />
            <PausedSection topics={topics.data.filter((t) => !t.isActive)} />
          </>
        )}
      </div>
    </div>
  );
}

function ActiveSection({ topics }: { topics: ReturnType<typeof useTopicsQuery>["data"] extends infer T ? T extends Array<infer U> ? U[] : never : never }) {
  if (topics.length === 0) return null;
  return (
    <>
      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-ink">On the radar</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">{topics.length} topics</div>
      </div>
      <div className="mb-16">
        <TopicGrid topics={topics} />
      </div>
    </>
  );
}

function PausedSection({ topics }: { topics: ReturnType<typeof useTopicsQuery>["data"] extends infer T ? T extends Array<infer U> ? U[] : never : never }) {
  if (topics.length === 0) return null;
  return (
    <>
      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-muted-foreground">Paused</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">{topics.length} topics</div>
      </div>
      <TopicGrid topics={topics} dim />
    </>
  );
}
