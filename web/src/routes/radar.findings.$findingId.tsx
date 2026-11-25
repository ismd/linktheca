import { useParams } from "react-router";
import { FindingReader } from "@/features/radar/components/FindingReader";

export default function FindingRoute() {
  const { findingId } = useParams();
  const id = Number(findingId);
  if (Number.isNaN(id) || id <= 0) {
    return <div className="p-8 font-body text-muted-foreground">Invalid article id.</div>;
  }
  return <FindingReader findingId={id} />;
}
