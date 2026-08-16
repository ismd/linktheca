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
  ...over,
});

describe("SourceRow", () => {
  it("hides admin actions from ordinary users", () => {
    render(
      <SourceRow
        feed={feed()}
        isAdmin={false}
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });

  it("toggles the subscription", async () => {
    const onToggle = vi.fn();
    render(
      <SourceRow
        feed={feed()}
        isAdmin={false}
        onToggle={onToggle}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    await userEvent.click(screen.getByRole("checkbox", { name: /the verge/i }));
    expect(onToggle).toHaveBeenCalledWith(true);
  });

  it("falls back to the hostname and surfaces fetch errors", () => {
    render(
      <SourceRow
        feed={feed({ title: null, lastError: "404 Not Found", lastFetchedAt: new Date() })}
        isAdmin
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
        isAdmin
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByText(/paused/i)).toBeInTheDocument();
  });
});
