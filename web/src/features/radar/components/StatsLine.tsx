import type { TopicWithStats } from "../types";

function fmtDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

type Props = {
  topic: TopicWithStats;
};

export function StatsLine({ topic }: Props) {
  const s = topic.stats;
  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-1 mt-6 mb-4">
      <span className="label-sc text-ink">
        <span className="text-vermillion font-bold">{s.totalCount}</span> found
      </span>
      <span className="label-sc text-muted-foreground">·</span>
      <span
        className={`label-sc ${s.newCount > 0 ? "text-vermillion" : "text-muted-foreground"}`}
      >
        {s.newCount} unread
      </span>
      <span className="label-sc text-muted-foreground">·</span>
      <span className="label-sc text-ink">{s.sourceCount} sources</span>
      <span className="label-sc text-muted-foreground">·</span>
      <span className="label-sc text-muted-foreground">
        created {fmtDate(topic.createdAt)}
      </span>
    </div>
  );
}
