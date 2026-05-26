import type { MatchView } from "../types";
import { MatchCard } from "./MatchCard";

type Props = {
  matches: MatchView[];
};

export function MatchGrid({ matches }: Props) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {matches.map((m, i) => (
        <MatchCard key={m.id} match={m} index={i} />
      ))}
    </div>
  );
}
