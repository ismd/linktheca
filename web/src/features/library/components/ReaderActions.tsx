import { useState } from "react";
import { toast } from "sonner";
import { ExternalLink, Star, Archive, Trash2, BookOpen, BookOpenCheck } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { useUpdateItem, useDeleteItem } from "../use-mutations";
import { DeleteConfirm } from "./DeleteConfirm";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
  onDeleted?: () => void;
};

export function ReaderActions({ item, onDeleted }: Props) {
  const update = useUpdateItem();
  const del = useDeleteItem();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const toggleFavorite = () =>
    update.mutate(
      { id: item.id, input: { isFavorite: !item.isFavorite } },
      {
        onError: () => toast.error("Couldn't update favorite"),
      },
    );

  const archive = () =>
    update.mutate(
      { id: item.id, input: { state: "archived" } },
      {
        onSuccess: () => toast.success("Archived"),
        onError: () => toast.error("Couldn't archive"),
      },
    );

  const toggleRead = () => {
    const next = item.state === "read" ? "unread" : "read";
    update.mutate(
      { id: item.id, input: { state: next } },
      {
        onError: () => toast.error("Couldn't update state"),
      },
    );
  };

  const confirmDelete = () => {
    del.mutate(item.id, {
      onSuccess: () => {
        toast.success("Deleted");
        setConfirmOpen(false);
        onDeleted?.();
      },
      onError: () => {
        toast.error("Couldn't delete");
        setConfirmOpen(false);
      },
    });
  };

  return (
    <div className="mt-16 pt-10 border-t-2 border-ink">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="outline" onClick={toggleRead}>
          {item.state === "read" ? (
            <BookOpen className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          ) : (
            <BookOpenCheck className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          )}
          {item.state === "read" ? "Mark as unread" : "Mark as read"}
        </Button>

        <Button variant="outline" onClick={toggleFavorite}>
          <Star
            className="h-4 w-4"
            strokeWidth={1.5}
            fill={item.isFavorite ? "currentColor" : "none"}
            aria-hidden="true"
          />
          {item.isFavorite ? "Favorited" : "Favorite"}
        </Button>

        <Button variant="outline" onClick={archive} disabled={item.state === "archived"}>
          <Archive className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          {item.state === "archived" ? "Archived" : "Archive"}
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
          onClick={() => setConfirmOpen(true)}
        >
          <Trash2 className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          Delete
        </Button>
      </div>

      <DeleteConfirm
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        onConfirm={confirmDelete}
        pending={del.isPending}
      />
    </div>
  );
}
