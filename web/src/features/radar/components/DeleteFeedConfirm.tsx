import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/ui/alert-dialog";
import type { FeedListItem } from "../types";

type Props = {
  feed: FeedListItem | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

function hostOf(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function DeleteFeedConfirm({ feed, pending, onOpenChange, onConfirm }: Props) {
  const name = feed ? (feed.title ?? hostOf(feed.url)) : "";

  return (
    <AlertDialog open={feed !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent className="paper-surface">
        {feed && (
          <>
            <AlertDialogHeader>
              <AlertDialogTitle className="display-tight text-2xl">
                Delete &ldquo;{name}&rdquo;?
              </AlertDialogTitle>
              <AlertDialogDescription className="font-body text-muted-foreground">
                {feed.findingCount} findings and their matches will be removed for all
                users. This cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
              <AlertDialogAction onClick={onConfirm} disabled={pending}>
                {pending ? "Deleting…" : "Delete"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </>
        )}
      </AlertDialogContent>
    </AlertDialog>
  );
}
