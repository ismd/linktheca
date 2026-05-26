import type { TopicWithStats } from "../types";
import { TopicCard } from "./TopicCard";

type Props = {
  topics: TopicWithStats[];
  dim?: boolean;
};

export function TopicGrid({ topics, dim = false }: Props) {
  return (
    <div
      className={`grid grid-cols-1 md:grid-cols-2 gap-5 ${dim ? "opacity-60" : ""}`}
    >
      {topics.map((t, i) => (
        <TopicCard key={t.id} topic={t} index={i} />
      ))}
    </div>
  );
}
