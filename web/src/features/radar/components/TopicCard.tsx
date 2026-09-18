import { Link } from "react-router";
import { fmtLastMatch } from "../time";
import type { TopicWithStats } from "../types";

type Props = {
  topic: TopicWithStats;
};

export function TopicCard({ topic }: Props) {
  const newCount = topic.stats.newCount;
  return (
    <Link
      to={`/radar/topics/${topic.id}`}
      className={`topic-card block p-6 ${topic.isActive ? "" : "inactive"} animate-fade-in`}
    >
      <div className="flex items-start justify-between gap-3 mb-3">
        <div className="label-sc text-muted-foreground">
          {topic.isActive ? null : "Paused"}
        </div>
        {newCount > 0 ? (
          <div className="label-sc text-vermillion">{newCount} new</div>
        ) : (
          <div className="label-sc text-muted-foreground">—</div>
        )}
      </div>
      <h3 className="card-title display-tight text-2xl text-ink leading-tight mb-3">
        {topic.name}
      </h3>
      <p className="font-body text-base text-muted-foreground leading-relaxed mb-5 line-clamp-2">
        {topic.description}
      </p>
      <div className="rule-dotted mb-4"></div>
      <div className="flex items-center justify-between">
        <div className="label-sc text-muted-foreground">
          {topic.stats.totalCount} found · {topic.stats.sourceCount} sources
        </div>
        <div className="label-sc text-muted-foreground">
          {fmtLastMatch(topic.stats.lastMatchAt)}
        </div>
      </div>
    </Link>
  );
}
