import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { MatchGrid } from "./MatchGrid";
import type { MatchView } from "../types";

const match = (id: number, topicName: string): MatchView => ({
  id,
  topicId: id,
  topicName,
  similarity: 0.7,
  state: "new",
  matchedAt: new Date("2026-05-18T10:00:00Z"),
  finding: {
    id: id + 100,
    feedId: 5,
    feedTitle: "Ink & Switch",
    url: `https://example.com/${id}`,
    title: `Title ${id}`,
    summary: null,
    publishedAt: null,
    discoveredAt: new Date("2026-05-18T09:00:00Z"),
  },
});

describe("MatchGrid", () => {
  it("passes showTopic down to every card", () => {
    render(
      <MemoryRouter>
        <MatchGrid matches={[match(1, "Rust"), match(2, "Postgres")]} showTopic />
      </MemoryRouter>,
    );
    expect(screen.getByText("Rust")).toBeInTheDocument();
    expect(screen.getByText("Postgres")).toBeInTheDocument();
  });

  it("does not render topic names without showTopic", () => {
    render(
      <MemoryRouter>
        <MatchGrid matches={[match(1, "Rust")]} />
      </MemoryRouter>,
    );
    expect(screen.queryByText("Rust")).toBeNull();
  });
});
