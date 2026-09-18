import { Button } from "@/shared/ui/button";

type Props = {
  message: string;
  onRetry: () => void;
};

export function ErrorPanel({ message, onRetry }: Props) {
  return (
    <div
      role="alert"
      className="border border-vermillion bg-paper-2 px-6 py-8 text-center"
    >
      <p className="font-body text-ink-3 mb-4">{message}</p>
      <Button variant="outline" onClick={onRetry}>
        Try again
      </Button>
    </div>
  );
}
