import { useParams } from "react-router";
import { MatchReader } from "@/features/radar/components/MatchReader";

export default function MatchRoute() {
  const { matchId } = useParams();
  const id = Number(matchId);
  if (Number.isNaN(id) || id <= 0) {
    return <div className="p-8 font-body text-muted-foreground">Invalid match id.</div>;
  }
  return <MatchReader matchId={id} />;
}
