import { Link } from "react-router";
import { ApiError } from "@/shared/api/errors";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { useTopicsQuery } from "@/features/radar/use-radar";
import { useNewTopicStore } from "@/features/radar/use-new-topic-store";
import { TopicGrid } from "@/features/radar/components/TopicGrid";
import { EmptyTopicList } from "@/features/radar/components/EmptyTopicList";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
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

export default function TopicsListRoute() {
  const topics = useTopicsQuery();
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
        title="Topics"
        subtitle="Everything on your radar"
        actions={
          <Link
            to="/radar/sources"
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Sources →
          </Link>
        }
      />
      <div className="px-4 lg:px-8 pb-6 pt-6">
        <Link
          to="/radar"
          className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-6"
        >
          ← Back to inbox
        </Link>
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
