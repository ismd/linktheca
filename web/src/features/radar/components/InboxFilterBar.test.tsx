import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InboxFilterBar } from "./InboxFilterBar";
import type { TopicWithStats } from "../types";

function topic(
  id: number,
  name: string,
  opts: { isActive?: boolean; newCount?: number } = {},
): TopicWithStats {
  return {
    id,
    userId: 1,
    name,
    description: "D",
    matchThreshold: 0.55,
    isActive: opts.isActive ?? true,
    hasEmbedding: true,
    createdAt: new Date("2026-05-01T10:00:00Z"),
    updatedAt: new Date("2026-05-02T10:00:00Z"),
    stats: {
      newCount: opts.newCount ?? 0,
      totalCount: 10,
      sourceCount: 2,
      lastMatchAt: null,
    },
  };
}

describe("InboxFilterBar", () => {
  it("renders All topics plus a chip per active topic with its new count", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 4 }), topic(2, "Postgres", { newCount: 2 })]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "All topics" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Rust4" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Postgres2" })).toBeInTheDocument();
  });

  it("hides the count when a topic has no new matches", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 0 })]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Rust" })).toBeInTheDocument();
    expect(screen.queryByText("0")).toBeNull();
  });

  it("shows a paused topic with unread matches and hides a paused topic without", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[
          topic(1, "Paused loud", { isActive: false, newCount: 3 }),
          topic(2, "Paused quiet", { isActive: false, newCount: 0 }),
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Paused loud3" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Paused quiet" })).toBeNull();
  });

  it("shows the selected topic even when it would otherwise be hidden", () => {
    render(
      <InboxFilterBar
        state="all"
        topicId={2}
        topics={[topic(2, "Paused quiet", { isActive: false, newCount: 0 })]}
        onChange={vi.fn()}
      />,
    );
    const chip = screen.getByRole("button", { name: "Paused quiet" });
    expect(chip).toHaveAttribute("aria-pressed", "true");
  });

  it("emits the topic id on click and keeps the current state", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="all"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 4 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Rust4" }));
    expect(onChange).toHaveBeenCalledWith({ state: "all", topicId: 1 });
  });

  it("clears the topic when the active chip is clicked again", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="new"
        topicId={1}
        topics={[topic(1, "Rust", { newCount: 4 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Rust4" }));
    expect(onChange).toHaveBeenCalledWith({ state: "new", topicId: undefined });
  });

  it("emits the new state while keeping the selected topic", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="new"
        topicId={3}
        topics={[topic(3, "Rust", { newCount: 1 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(onChange).toHaveBeenCalledWith({ state: "all", topicId: 3 });
  });

  it("marks the active state button with aria-pressed", () => {
    render(
      <InboxFilterBar state="new" topicId={undefined} topics={[]} onChange={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "New" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "All" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });
});
