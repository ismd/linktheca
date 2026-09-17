import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SourceRow } from "./SourceRow";
import type { FeedListItem } from "../types";

const feed = (over: Partial<FeedListItem> = {}): FeedListItem => ({
  id: 1,
  url: "https://theverge.com/rss",
  kind: "rss",
  title: "The Verge",
  fetchIntervalSeconds: 3600,
  isActive: true,
  lastFetchedAt: null,
  lastError: null,
  createdAt: new Date("2026-08-01T10:00:00Z"),
  subscribed: false,
  findingCount: 214,
  isOwn: false,
  ...over,
});

describe("SourceRow", () => {
  it("hides the management actions when the row is not manageable", () => {
    render(<SourceRow feed={feed()} canManage={false} onToggle={() => {}} />);
    expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });

  it("shows them when it is", () => {
    render(
      <SourceRow
        feed={feed({ isOwn: true })}
        canManage
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
  });

  it("omits the checkbox when subscription is not offered", () => {
    render(<SourceRow feed={feed()} canManage onEdit={() => {}} onDelete={() => {}} />);
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.getByText("The Verge")).toBeInTheDocument();
  });

  it("toggles the subscription", async () => {
    const onToggle = vi.fn();
    render(<SourceRow feed={feed()} canManage={false} onToggle={onToggle} />);
    await userEvent.click(screen.getByRole("checkbox", { name: /the verge/i }));
    expect(onToggle).toHaveBeenCalledWith(true);
  });

  it("falls back to the hostname and surfaces fetch errors", () => {
    render(
      <SourceRow
        feed={feed({ title: null, lastError: "404 Not Found", lastFetchedAt: new Date() })}
        canManage
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByText("theverge.com")).toBeInTheDocument();
    expect(screen.getByText(/404 Not Found/)).toBeInTheDocument();
  });

  it("marks a paused feed", () => {
    render(
      <SourceRow
        feed={feed({ isActive: false })}
        canManage
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByText(/paused/i)).toBeInTheDocument();
  });
});
