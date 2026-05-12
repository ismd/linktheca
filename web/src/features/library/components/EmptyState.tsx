import { Button } from "@/shared/ui/button";
import { useAddLinkStore } from "../use-add-link-store";

type Props = {
  filtered: boolean;
};

export function EmptyState({ filtered }: Props) {
  const open = useAddLinkStore((s) => s.open);

  if (filtered) {
    return (
      <div className="text-center py-20 border border-dashed border-rule">
        <p className="label-sc text-muted-foreground mb-3">No matches</p>
        <p className="font-body italic text-ink-3">
          No items match the current filters.
        </p>
      </div>
    );
  }

  return (
    <div className="text-center py-24">
      <h2 className="display-tight text-4xl text-ink mb-3">Nothing here yet</h2>
      <p className="label-sc text-muted-foreground mb-8">Save your first link →</p>
      <Button onClick={open}>+ Add link</Button>
    </div>
  );
}
