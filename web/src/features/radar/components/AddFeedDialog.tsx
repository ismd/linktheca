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
import { ApiError } from "@/shared/api/errors";
import { useAddFeed } from "../use-mutations";

const schema = z.object({
  url: z.string().url("Enter a valid http(s) URL"),
  fetchIntervalSeconds: z.coerce.number().int(),
});
// The interval comes off a native <select> as a string, so the schema coerces
// it. That makes the form's input and output types differ.
type FormInput = z.input<typeof schema>;
type FormValues = z.output<typeof schema>;

export const INTERVAL_OPTIONS = [
  { value: 1800, label: "every 30m" },
  { value: 3600, label: "every 1h" },
  { value: 10800, label: "every 3h" },
  { value: 21600, label: "every 6h" },
  { value: 43200, label: "every 12h" },
  { value: 86400, label: "every 24h" },
];

export function mapFeedError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 409) return "This feed is already in the catalog";
    if (err.status === 400) return err.message || "Invalid input";
    if (err.status === 403) return "Only an instance admin can add feeds";
  }
  return "Could not save — please try again";
}

function AddFeedForm({ onClose }: { onClose: () => void }) {
  const add = useAddFeed();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { url: "", fetchIntervalSeconds: 3600 },
  });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async ({ url, fetchIntervalSeconds }) => {
    setTopError(null);
    try {
      await add.mutateAsync({ url, fetchIntervalSeconds });
      toast.success("Feed added");
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
        <Label htmlFor="add-feed-url" className="label-sc text-ink-3">
          Feed URL
        </Label>
        <Input
          id="add-feed-url"
          aria-invalid={errors.url ? "true" : "false"}
          disabled={add.isPending}
          {...register("url")}
        />
        {errors.url && (
          <p className="text-sm text-vermillion-dark">{errors.url.message}</p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="add-feed-interval" className="label-sc text-ink-3">
          Check
        </Label>
        <select
          id="add-feed-interval"
          className="border border-rule bg-paper px-3 py-2 font-body text-base focus:outline-none focus:ring-2 focus:ring-ink/10"
          disabled={add.isPending}
          {...register("fetchIntervalSeconds")}
        >
          {INTERVAL_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          disabled={add.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={add.isPending}>
          {add.isPending ? "Adding…" : "Add feed"}
        </Button>
      </DialogFooter>
    </form>
  );
}

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function AddFeedDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="paper-surface max-h-[85dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Add feed</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Everyone on this instance can subscribe to it.
          </DialogDescription>
        </DialogHeader>
        {open && <AddFeedForm onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
