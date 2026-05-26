import { Button } from "@/shared/ui/button";
import type { TopicWithStats } from "../types";

type Props = {
  topic: TopicWithStats;
  onEdit: () => void;
  onTogglePause: () => void;
  onDelete: () => void;
  togglePending: boolean;
};

export function TopicHeader({
  topic,
  onEdit,
  onTogglePause,
  onDelete,
  togglePending,
}: Props) {
  return (
    <div className="flex items-start justify-between gap-6 mb-4">
      <div className="flex-1 min-w-0">
        <div className="label-sc-lg text-muted-foreground mb-3">
          {topic.isActive ? "Standing watch" : "Paused"}
        </div>
        <h1 className="display-tight text-4xl md:text-5xl text-ink leading-tight">
          {topic.name}
        </h1>
        <p className="mt-4 font-body text-lg text-ink-3 leading-relaxed max-w-[680px]">
          {topic.description}
        </p>
      </div>
      <div className="hidden md:flex gap-2 flex-shrink-0 pt-1">
        <Button variant="outline" onClick={onEdit} aria-label="Edit topic">
          Edit
        </Button>
        <Button
          variant="ghost"
          onClick={onTogglePause}
          disabled={togglePending}
          aria-label={topic.isActive ? "Pause topic" : "Resume topic"}
        >
          {topic.isActive ? "Pause" : "Resume"}
        </Button>
        <Button variant="ghost" onClick={onDelete} aria-label="Delete topic">
          Delete
        </Button>
      </div>
    </div>
  );
}
