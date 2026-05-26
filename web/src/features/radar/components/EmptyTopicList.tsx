import { Button } from "@/shared/ui/button";

type Props = {
  onAdd: () => void;
};

export function EmptyTopicList({ onAdd }: Props) {
  return (
    <div className="text-center py-20 border border-dashed border-rule">
      <p className="display-tight text-3xl text-ink mb-3">Nothing on your radar yet</p>
      <p className="font-body italic text-muted-foreground mb-8">
        Add your first topic to start watching for new signals.
      </p>
      <Button onClick={onAdd}>+ New topic</Button>
    </div>
  );
}
