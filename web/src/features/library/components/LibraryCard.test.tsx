import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
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
  image: null,
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

  it("shows no stamp when the item is unread", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    expect(screen.queryByText(/saved/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/✓ read/i)).not.toBeInTheDocument();
  });

  it("shows the downloaded preview image when the item has one", () => {
    const { container } = render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, image: "a1b2c3.png" }} />
      </MemoryRouter>,
    );
    const img = container.querySelector("img");
    expect(img).toBeInTheDocument();
    // Served off /media, not through the /api prefix
    expect(img).toHaveAttribute("src", "/media/images/a1b2c3.png");
    // Decorative: the title right below already names the article
    expect(img).toHaveAttribute("alt", "");
  });

  it("renders no image element when the item has no preview", () => {
    const { container } = render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    expect(container.querySelector("img")).not.toBeInTheDocument();
  });

  it("drops a preview that fails to load, leaving the card intact", () => {
    const { container } = render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, image: "missing.png" }} />
      </MemoryRouter>,
    );

    const img = container.querySelector("img")!;
    fireEvent.error(img);

    expect(container.querySelector("img")).not.toBeInTheDocument();
    expect(screen.getByText("Example Title")).toBeInTheDocument();
  });

  it("falls back to URL when title is null", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, title: null }} />
      </MemoryRouter>,
    );
    expect(screen.getByText("https://example.com/article")).toBeInTheDocument();
  });

  it("breaks a long unspaced title so it cannot overflow the card", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, title: null }} />
      </MemoryRouter>,
    );
    // A URL fallback is one unbreakable word; without break-words it spills
    // over the neighbouring grid columns. jsdom has no layout, so this pins
    // the class rather than the rendered width.
    const heading = screen.getByRole("heading", { level: 2 });
    expect(heading.className).toMatch(/\bbreak-words\b/);
  });
});
