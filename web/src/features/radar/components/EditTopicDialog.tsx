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
import { useUpdateTopic } from "../use-mutations";
import type { TopicWithStats } from "../types";
import type { UpdateTopicInput } from "../api";

const schema = z.object({
  name: z.string().min(1, "Name is required").max(200),
  description: z.string().min(10, "Description must be at least 10 characters").max(2000),
});
type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "embedder_unavailable") {
      return "Embedder is currently unavailable. Try again in a moment.";
    }
    if (err.status === 400) return err.message || "Invalid input";
    return "Could not save — please try again";
  }
  return "Could not save — please try again";
}

type Props = {
  topic: TopicWithStats;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function EditTopicForm({ topic, onClose }: { topic: TopicWithStats; onClose: () => void }) {
  const update = useUpdateTopic();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: topic.name, description: topic.description },
  });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    const patch: UpdateTopicInput = {};
    if (values.name !== topic.name) patch.name = values.name;
    if (values.description !== topic.description) patch.description = values.description;
    if (Object.keys(patch).length === 0) {
      onClose();
      return;
    }
    try {
      await update.mutateAsync({ id: topic.id, input: patch });
      toast.success("Saved");
      onClose();
    } catch (err) {
      setTopError(mapError(err));
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
        <Label htmlFor="edit-topic-name" className="label-sc text-ink-3">Name</Label>
        <Input
          id="edit-topic-name"
          aria-invalid={errors.name ? "true" : "false"}
          disabled={update.isPending}
          {...register("name")}
        />
        {errors.name && (
          <p className="text-sm text-vermillion-dark">{errors.name.message}</p>
        )}
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="edit-topic-desc" className="label-sc text-ink-3">Description</Label>
        <textarea
          id="edit-topic-desc"
          rows={4}
          className="border border-rule bg-paper px-3 py-2 font-body text-base focus:outline-none focus:ring-2 focus:ring-ink/10"
          aria-invalid={errors.description ? "true" : "false"}
          disabled={update.isPending}
          {...register("description")}
        />
        {errors.description && (
          <p className="text-sm text-vermillion-dark">{errors.description.message}</p>
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={update.isPending}>
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function EditTopicDialog({ topic, open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="paper-surface">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Edit topic</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Changing the description will re-embed the topic.
          </DialogDescription>
        </DialogHeader>
        {open && <EditTopicForm topic={topic} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
