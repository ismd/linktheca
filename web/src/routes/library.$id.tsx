import { useParams } from "react-router";

export default function LibraryItemRoute() {
  const { id } = useParams();
  return (
    <div className="p-8">
      <h1 className="font-display text-3xl text-ink">Reader: {id}</h1>
    </div>
  );
}
