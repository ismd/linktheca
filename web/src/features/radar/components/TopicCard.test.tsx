import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { TopicCard } from "./TopicCard";
import type { TopicWithStats } from "../types";

const topic: TopicWithStats = {
  id: 7, userId: 1, name: "Local-first software", description: "CRDTs and beyond",
  matchThreshold: 0.55, isActive: true, hasEmbedding: true,
  createdAt: new Date("2026-04-01"), updatedAt: new Date("2026-05-01"),
  stats: { newCount: 3, totalCount: 21, sourceCount: 4, lastMatchAt: new Date() },
};

function r(node: React.ReactElement) {
  return render(<MemoryRouter>{node}</MemoryRouter>);
}

describe("TopicCard", () => {
  it("renders name, description, stats and link", () => {
    r(<TopicCard topic={topic} index={0} />);
    expect(screen.getByText("Local-first software")).toBeInTheDocument();
    expect(screen.getByText(/CRDTs and beyond/)).toBeInTheDocument();
    expect(screen.getByText("3 new")).toBeInTheDocument();
    expect(screen.getByText(/21 found/)).toBeInTheDocument();
    expect(screen.getByText(/4 sources/)).toBeInTheDocument();
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/radar/topics/7");
  });

  it("renders dash when newCount is 0", () => {
    r(<TopicCard topic={{ ...topic, stats: { ...topic.stats, newCount: 0 } }} index={1} />);
    expect(screen.queryByText("0 new")).toBeNull();
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
