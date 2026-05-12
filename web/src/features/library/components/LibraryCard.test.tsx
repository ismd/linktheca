import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { LibraryCard } from "./LibraryCard";
import type { LibraryItem } from "../types";

const baseItem: LibraryItem = {
  id: 7,
  state: "unread",
  isFavorite: false,
  note: null,
  savedAt: new Date("2026-05-10T12:00:00Z"),
  readAt: null,
  url: "https://example.com/article",
  title: "Example Title",
  excerpt: "Some short excerpt",
  readingTimeSeconds: 180,
};

describe("LibraryCard", () => {
  it("renders title, excerpt, reading-time and host link", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    expect(screen.getByText("Example Title")).toBeInTheDocument();
    expect(screen.getByText(/Some short excerpt/)).toBeInTheDocument();
    expect(screen.getByText(/3 min read/)).toBeInTheDocument();
    expect(screen.getByText(/example\.com/)).toBeInTheDocument();
  });

  it("wraps the entire card in a Link to /library/:id", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/library/7");
  });

  it("shows the favorite mark when isFavorite=true", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, isFavorite: true }} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText(/favorite/i)).toBeInTheDocument();
  });

  it("shows the read stamp when state='read'", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, state: "read" }} />
      </MemoryRouter>,
    );
    expect(screen.getByText(/✓ read/i)).toBeInTheDocument();
  });

  it("falls back to URL when title is null", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, title: null }} />
      </MemoryRouter>,
    );
    expect(screen.getByText("https://example.com/article")).toBeInTheDocument();
  });
});
