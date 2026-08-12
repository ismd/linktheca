import {
  MoreHorizontal,
  Star,
  Archive,
  ArchiveRestore,
  BookOpen,
  BookOpenCheck,
  Trash2,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/shared/ui/dropdown-menu";
import { useItemActions } from "../use-item-actions";
import { DeleteConfirm } from "./DeleteConfirm";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
};

export function LibraryCardMenu({ item }: Props) {
  const actions = useItemActions(item);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          aria-label="Article actions"
          className="h-7 w-7 flex items-center justify-center text-muted-foreground hover:bg-paper-2 hover:text-ink outline-none focus-visible:ring-2 focus-visible:ring-ink-3"
        >
          <MoreHorizontal className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
        </DropdownMenuTrigger>

        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={actions.toggleRead}>
            {actions.isRead ? (
              <BookOpen className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            ) : (
              <BookOpenCheck className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            )}
            {actions.isRead ? "Mark as unread" : "Mark as read"}
          </DropdownMenuItem>

          <DropdownMenuItem onSelect={actions.toggleFavorite}>
            <Star
              className="h-4 w-4"
              strokeWidth={1.5}
              fill={item.isFavorite ? "currentColor" : "none"}
              aria-hidden="true"
            />
            {item.isFavorite ? "Unfavorite" : "Favorite"}
          </DropdownMenuItem>

          <DropdownMenuItem onSelect={actions.toggleArchive}>
            {actions.isArchived ? (
              <ArchiveRestore className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            ) : (
              <Archive className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            )}
            {actions.isArchived ? "Unarchive" : "Archive"}
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            onSelect={actions.requestDelete}
            className="text-vermillion-dark data-[highlighted]:text-vermillion"
          >
            <Trash2 className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <DeleteConfirm
        open={actions.confirmOpen}
        onOpenChange={actions.setConfirmOpen}
        onConfirm={actions.confirmDelete}
        pending={actions.deletePending}
      />
    </>
  );
}
