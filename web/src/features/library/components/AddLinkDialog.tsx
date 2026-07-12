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
import { useAddLinkStore } from "../use-add-link-store";
import { useSaveLink } from "../use-mutations";

const schema = z.object({
  url: z.string().url("Please enter a valid URL (including https://)"),
});
type FormValues = z.infer<typeof schema>;

const PROGRESS_STAGES = [
  "Fetching page…",
  "Extracting content…",
  "Saving to library…",
];

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "already_saved" || err.status === 409) {
      return "This article is already in your library";
    }
    if (err.status === 422) {
      return "Couldn't extract content from this URL";
    }
    if (err.status >= 500) {
      return "Couldn't save — please try again";
    }
    return err.message || "Couldn't save — please try again";
  }
  return "Couldn't save — please try again";
}

function AddLinkForm({ onClose }: { onClose: () => void }) {
  const save = useSaveLink();

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { url: "" },
  });

  const [topError, setTopError] = useState<string | null>(null);
  const [stage, setStage] = useState(0);

  const onSubmit = handleSubmit(async ({ url }) => {
    setTopError(null);
    setStage(0);
    const t1 = setTimeout(() => setStage(1), 1500);
    const t2 = setTimeout(() => setStage(2), 3500);
    try {
      await save.mutateAsync(url);
      toast.success("Saved to library");
      onClose();
    } catch (err) {
      setTopError(mapError(err));
    } finally {
      clearTimeout(t1);
      clearTimeout(t2);
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
        <Label htmlFor="add-link-url" className="label-sc text-ink-3">
          URL
        </Label>
        <Input
          id="add-link-url"
          placeholder="https://…"
          aria-invalid={errors.url ? "true" : "false"}
          aria-describedby={errors.url ? "add-link-url-error" : undefined}
          disabled={save.isPending}
          {...register("url")}
        />
        {errors.url && (
          <p
            id="add-link-url-error"
            className="text-sm text-vermillion-dark"
          >
            {errors.url.message}
          </p>
        )}
      </div>

      {save.isPending && (
        <div className="rounded border border-rule bg-paper-2/50 px-4 py-3">
          <p className="label-sc text-ink-3">{PROGRESS_STAGES[stage]}</p>
        </div>
      )}

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          disabled={save.isPending}
        >
          Cancel
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function AddLinkDialog() {
  const isOpen = useAddLinkStore((s) => s.isOpen);
  const close = useAddLinkStore((s) => s.close);

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent className="paper-surface">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Add a link</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Paste a URL — fetch and save the article
          </DialogDescription>
        </DialogHeader>

        {isOpen && <AddLinkForm onClose={close} />}
      </DialogContent>
    </Dialog>
  );
}
