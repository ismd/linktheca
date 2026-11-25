import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";
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
import { Label } from "@/shared/ui/label";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useNewTopicStore } from "../use-new-topic-store";
import { useCreateTopic } from "../use-mutations";
import { TopicPreviewPanel } from "./TopicPreviewPanel";
import { TOPIC_DESCRIPTION_PLACEHOLDER, TopicWritingTips } from "./TopicWritingTips";

const schema = z.object({
  description: z
    .string()
    .trim()
    .min(10, "Write at least 10 characters")
    .max(2000, "Too long — keep it to a few sentences"),
});
type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "embedder_unavailable") {
      return "Embedder is currently unavailable. Try again in a moment.";
    }
    if (err.status === 400) {
      return err.message || "Invalid input";
    }
    return "Could not save — please try again";
  }
  return "Could not save — please try again";
}

function NewTopicForm({ onClose }: { onClose: () => void }) {
  const create = useCreateTopic();
  const {
    register,
    handleSubmit,
    control,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { description: "" },
  });
  const draftDescription = useWatch({ control, name: "description" });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async ({ description }) => {
    setTopError(null);
    try {
      await create.mutateAsync({ description });
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
        <Label htmlFor="new-topic-desc" className="label-sc text-ink-3">
          What to watch for
        </Label>
        <textarea
          id="new-topic-desc"
          rows={4}
          placeholder={TOPIC_DESCRIPTION_PLACEHOLDER}
          className="border border-rule bg-paper px-3 py-2 font-body text-base placeholder:text-muted-foreground/70 focus:outline-none focus:ring-2 focus:ring-ink/10"
          aria-invalid={errors.description ? "true" : "false"}
          aria-describedby="new-topic-tips"
          disabled={create.isPending}
          {...register("description")}
        />
        {errors.description && (
          <p className="text-sm text-vermillion-dark">{errors.description.message}</p>
        )}
        <TopicWritingTips id="new-topic-tips" />
      </div>

      <TopicPreviewPanel description={draftDescription} />

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={create.isPending}>
          Cancel
        </Button>
        <Button type="submit" disabled={create.isPending}>
          {create.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function NewTopicDialog() {
  const isOpen = useNewTopicStore((s) => s.isOpen);
  const close = useNewTopicStore((s) => s.close);

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent className="paper-surface max-h-[85dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">New topic</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            A sentence or two is enough. The first words become its name — rename it any time.
          </DialogDescription>
        </DialogHeader>
        {isOpen && <NewTopicForm onClose={close} />}
      </DialogContent>
    </Dialog>
  );
}
