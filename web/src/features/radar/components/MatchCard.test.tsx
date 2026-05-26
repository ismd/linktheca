import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { MatchCard } from "./MatchCard";
import type { MatchView } from "../types";

const match: MatchView = {
  id: 42, topicId: 7, topicName: "Local-first", similarity: 0.7,
  state: "new", matchedAt: new Date("2026-05-18T10:00:00Z"),
  finding: {
    id: 100, feedId: 5, feedTitle: "Ink & Switch",
    url: "https://inkandswitch.com/local-first/",
    title: "Local-First Software", summary: "A great essay…",
    publishedAt: new Date("2026-05-17T10:00:00Z"),
    discoveredAt: new Date("2026-05-18T09:00:00Z"),
  },
};

function r(n: React.ReactElement) {
  return render(<MemoryRouter>{n}</MemoryRouter>);
}

describe("MatchCard", () => {
  it("renders title, source, new-stamp, and link", () => {
    r(<MatchCard match={match} index={0} />);
    expect(screen.getByText("Local-First Software")).toBeInTheDocument();
    expect(screen.getByText(/Ink & Switch/)).toBeInTheDocument();
    expect(screen.getByText("new")).toBeInTheDocument();
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/radar/matches/42");
  });

  it("hides new-stamp when state is seen", () => {
    r(<MatchCard match={{ ...match, state: "seen" }} index={0} />);
    expect(screen.queryByText("new")).toBeNull();
  });

  it("falls back to URL when finding.title is null", () => {
    r(<MatchCard match={{ ...match, finding: { ...match.finding, title: null } }} index={0} />);
    expect(screen.getByText(/inkandswitch.com/)).toBeInTheDocument();
  });
});
