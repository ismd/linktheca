import { useState } from "react";
import { toast } from "sonner";
import { useUpdateItem, useDeleteItem } from "./use-mutations";
import type { LibraryItem, LibraryState } from "./types";

type Options = {
  onDeleted?: () => void;
  /** Lets the reader stand down its scroll-based auto-marking after a manual toggle. */
  onReadStateToggled?: () => void;
};

/**
 * The read/favorite/archive/delete semantics shared by the reader footer and
 * the card menu, so the two cannot drift apart.
 */
export function useItemActions(item: LibraryItem, options: Options = {}) {
  const update = useUpdateItem();
  const del = useDeleteItem();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const isArchived = item.state === "archived";
  const isRead = item.state === "read";

  const patchState = (state: LibraryState, success: string, failure: string) =>
    update.mutate(
      { id: item.id, input: { state } },
      {
        onSuccess: () => toast.success(success),
        onError: () => toast.error(failure),
      },
    );

  const toggleRead = () => {
    options.onReadStateToggled?.();
    if (isRead) {
      patchState("unread", "Marked as unread", "Couldn't update state");
    } else {
      patchState("read", "Marked as read", "Couldn't update state");
    }
  };

  const toggleArchive = () => {
    if (isArchived) {
      patchState("unread", "Moved to unread", "Couldn't unarchive");
    } else {
      patchState("archived", "Archived", "Couldn't archive");
    }
  };

  // No success toast: the card and reader both show the favorite mark already.
  const toggleFavorite = () =>
    update.mutate(
      { id: item.id, input: { isFavorite: !item.isFavorite } },
      { onError: () => toast.error("Couldn't update favorite") },
    );

  const requestDelete = () => setConfirmOpen(true);

  const confirmDelete = () =>
    del.mutate(item.id, {
      onSuccess: () => {
        toast.success("Deleted");
        setConfirmOpen(false);
        options.onDeleted?.();
      },
      onError: () => {
        toast.error("Couldn't delete");
        setConfirmOpen(false);
      },
    });

  return {
    isArchived,
    isRead,
    toggleRead,
    toggleArchive,
    toggleFavorite,
    requestDelete,
    confirmDelete,
    confirmOpen,
    setConfirmOpen,
    deletePending: del.isPending,
  };
}
