import type { ReactNode } from "react";

type Props = {
  pending: boolean;
  children: ReactNode;
};

// Wraps a list that is still showing the previous filter's results while the
// next request is in flight. Nothing leaves the page: the region only dims and
// reports itself busy, so a filter change reads as a refresh, not a reload.
export function PendingRegion({ pending, children }: Props) {
  return (
    <div
      aria-busy={pending}
      data-testid="pending-region"
      className={`transition-opacity duration-200 motion-reduce:transition-none ${
        pending ? "opacity-55" : "opacity-100"
      }`}
    >
      {children}
    </div>
  );
}
