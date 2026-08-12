import {
  ExternalLink,
  Star,
  Archive,
  ArchiveRestore,
  Trash2,
  BookOpen,
  BookOpenCheck,
} from "lucide-react";
import { Button } from "@/shared/ui/button";
import { useItemActions } from "../use-item-actions";
import { DeleteConfirm } from "./DeleteConfirm";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
  onDeleted?: () => void;
  onReadStateToggled?: () => void;
};

export function ReaderActions({ item, onDeleted, onReadStateToggled }: Props) {
  const actions = useItemActions(item, { onDeleted, onReadStateToggled });

  return (
    <div className="mt-16 pt-10 border-t-2 border-ink">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="outline" onClick={actions.toggleRead}>
          {actions.isRead ? (
            <BookOpen className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          ) : (
            <BookOpenCheck className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          )}
          {actions.isRead ? "Mark as unread" : "Mark as read"}
        </Button>

        <Button variant="outline" onClick={actions.toggleFavorite}>
          <Star
            className="h-4 w-4"
            strokeWidth={1.5}
            fill={item.isFavorite ? "currentColor" : "none"}
            aria-hidden="true"
          />
          {item.isFavorite ? "Favorited" : "Favorite"}
        </Button>

        <Button variant="outline" onClick={actions.toggleArchive}>
          {actions.isArchived ? (
            <ArchiveRestore className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          ) : (
            <Archive className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          )}
          {actions.isArchived ? "Unarchive" : "Archive"}
        </Button>

        <Button variant="ghost" asChild>
          <a href={item.url} target="_blank" rel="noopener noreferrer">
            <ExternalLink className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            Open original
          </a>
        </Button>

        <Button
          variant="ghost"
          className="ml-auto text-vermillion-dark hover:text-vermillion"
          onClick={actions.requestDelete}
        >
          <Trash2 className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          Delete
        </Button>
      </div>

      <DeleteConfirm
        open={actions.confirmOpen}
        onOpenChange={actions.setConfirmOpen}
        onConfirm={actions.confirmDelete}
        pending={actions.deletePending}
      />
    </div>
  );
}
