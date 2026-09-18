import { Link } from "react-router";
import { relativeFromNow } from "@/features/library/time";
import type { MatchView } from "../types";

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

type Props = {
  match: MatchView;
  showTopic?: boolean;
};

export function MatchCard({ match, showTopic = false }: Props) {
  const f = match.finding;
  const title = f.title ?? host(f.url);
  const source = f.feedTitle ?? host(f.url);
  const stamp = match.state === "new";
  const when = f.publishedAt ?? f.discoveredAt;
  return (
    <Link to={`/radar/matches/${match.id}`} className="feed-card group block">
      <article className="flex flex-col h-full p-5 border border-rule">
        <div className="flex items-center gap-x-3 gap-y-1.5 mb-3 flex-wrap">
          {stamp && <span className="stamp text-vermillion stamp-flat">new</span>}
          {showTopic && (
            <span className="label-sc text-ink">{match.topicName}</span>
          )}
          <span className="label-sc text-muted-foreground">
            {source} · {relativeFromNow(when)}
          </span>
        </div>
        <h2 className="card-title display-tight text-xl text-ink leading-tight mb-3 line-clamp-2">
          {title}
        </h2>
        {f.summary && (
          <p className="font-body text-base text-muted-foreground leading-relaxed line-clamp-3">
            {f.summary}
          </p>
        )}
      </article>
    </Link>
  );
}
