import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Button } from "@/shared/ui/button";
import { useUpdateFeed } from "../use-mutations";
import { INTERVAL_OPTIONS, mapFeedError } from "./AddFeedDialog";
import type { FeedListItem } from "../types";

const schema = z.object({
  title: z.string().max(500, "Name too long"),
  fetchIntervalSeconds: z.coerce.number().int(),
  paused: z.boolean(),
});
type FormInput = z.input<typeof schema>;
type FormValues = z.output<typeof schema>;

function EditFeedForm({ feed, onClose }: { feed: FeedListItem; onClose: () => void }) {
  const update = useUpdateFeed();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      title: feed.title ?? "",
      fetchIntervalSeconds: feed.fetchIntervalSeconds,
      paused: !feed.isActive,
    },
  });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async ({ title, fetchIntervalSeconds, paused }) => {
    setTopError(null);
    try {
      // An empty title is sent as "" on purpose: it clears the manual name and
      // lets the crawler fill it from the channel again.
      await update.mutateAsync({
        id: feed.id,
        input: { title, fetchIntervalSeconds, isActive: !paused },
      });
      toast.success("Feed updated");
      onClose();
    } catch (err) {
      setTopError(mapFeedError(err));
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-3 py-2 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="edit-feed-title" className="label-sc text-ink-3">
          Name
        </Label>
        <Input
          id="edit-feed-title"
          aria-invalid={errors.title ? "true" : "false"}
          disabled={update.isPending}
          {...register("title")}
        />
        <p className="label-sc text-muted-foreground">
          Leave empty to use the feed&rsquo;s own title.
        </p>
        {errors.title && (
          <p className="text-sm text-vermillion-dark">{errors.title.message}</p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="edit-feed-interval" className="label-sc text-ink-3">
          Check
        </Label>
        <select
          id="edit-feed-interval"
          className="border border-rule bg-paper px-3 py-2 font-body text-base focus:outline-none focus:ring-2 focus:ring-ink/10"
          disabled={update.isPending}
          {...register("fetchIntervalSeconds")}
        >
          {INTERVAL_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      <div className="flex items-center gap-2">
        <input
          id="edit-feed-paused"
          type="checkbox"
          className="h-4 w-4 accent-vermillion"
          disabled={update.isPending}
          {...register("paused")}
        />
        <Label htmlFor="edit-feed-paused" className="label-sc text-ink-3">
          Paused
        </Label>
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          disabled={update.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

type Props = {
  feed: FeedListItem | null;
  onOpenChange: (open: boolean) => void;
};

export function EditFeedDialog({ feed, onOpenChange }: Props) {
  return (
    <Dialog open={feed !== null} onOpenChange={onOpenChange}>
      <DialogContent className="paper-surface max-h-[85dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Edit feed</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            {feed?.url}
          </DialogDescription>
        </DialogHeader>
        {feed && <EditFeedForm feed={feed} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
