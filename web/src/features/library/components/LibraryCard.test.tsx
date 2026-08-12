import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Routes, Route } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { LibraryCard } from "./LibraryCard";
import type { LibraryItem } from "../types";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

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
    render(<LibraryCard item={baseItem} />, { wrapper: wrapper() });
    expect(screen.getByText("Example Title")).toBeInTheDocument();
    expect(screen.getByText(/Some short excerpt/)).toBeInTheDocument();
    expect(screen.getByText(/3 min read/)).toBeInTheDocument();
    expect(screen.getByText(/example\.com/)).toBeInTheDocument();
  });

  it("wraps the entire card in a Link to /library/:id", () => {
    render(<LibraryCard item={baseItem} />, { wrapper: wrapper() });
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/library/7");
  });

  it("shows the favorite mark when isFavorite=true", () => {
    render(<LibraryCard item={{ ...baseItem, isFavorite: true }} />, {
      wrapper: wrapper(),
    });
    expect(screen.getByLabelText(/favorite/i)).toBeInTheDocument();
  });

  it("shows the read stamp when state='read'", () => {
    render(<LibraryCard item={{ ...baseItem, state: "read" }} />, {
      wrapper: wrapper(),
    });
    expect(screen.getByText(/✓ read/i)).toBeInTheDocument();
  });

  it("shows the saved stamp when the item is unread", () => {
    render(<LibraryCard item={baseItem} />, { wrapper: wrapper() });
    expect(screen.getByText(/saved/i)).toBeInTheDocument();
    expect(screen.queryByText(/✓ read/i)).not.toBeInTheDocument();
  });

  it("shows the downloaded preview image when the item has one", () => {
    const { container } = render(
      <LibraryCard item={{ ...baseItem, image: "a1b2c3.png" }} />,
      { wrapper: wrapper() },
    );
    const img = container.querySelector("img");
    expect(img).toBeInTheDocument();
    // Served off /media, not through the /api prefix
    expect(img).toHaveAttribute("src", "/media/images/a1b2c3.png");
    // Decorative: the title right below already names the article
    expect(img).toHaveAttribute("alt", "");
  });

  it("renders no image element when the item has no preview", () => {
    const { container } = render(<LibraryCard item={baseItem} />, {
      wrapper: wrapper(),
    });
    expect(container.querySelector("img")).not.toBeInTheDocument();
  });

  it("drops a preview that fails to load, leaving the card intact", () => {
    const { container } = render(
      <LibraryCard item={{ ...baseItem, image: "missing.png" }} />,
      { wrapper: wrapper() },
    );

    const img = container.querySelector("img")!;
    fireEvent.error(img);

    expect(container.querySelector("img")).not.toBeInTheDocument();
    expect(screen.getByText("Example Title")).toBeInTheDocument();
  });

  it("falls back to URL when title is null", () => {
    render(<LibraryCard item={{ ...baseItem, title: null }} />, {
      wrapper: wrapper(),
    });
    expect(screen.getByText("https://example.com/article")).toBeInTheDocument();
  });

  it("breaks a long unspaced title so it cannot overflow the card", () => {
    render(<LibraryCard item={{ ...baseItem, title: null }} />, {
      wrapper: wrapper(),
    });
    // A URL fallback is one unbreakable word; without break-words it spills
    // over the neighbouring grid columns. jsdom has no layout, so this pins
    // the class rather than the rendered width.
    const heading = screen.getByRole("heading", { level: 2 });
    expect(heading.className).toMatch(/\bbreak-words\b/);
  });

  it("carries an actions menu", async () => {
    render(<LibraryCard item={baseItem} />, { wrapper: wrapper() });

    await userEvent.click(
      screen.getByRole("button", { name: /article actions/i }),
    );

    expect(await screen.findByRole("menu")).toBeInTheDocument();
  });

  it("puts the actions menu on the meta line rather than over the preview", () => {
    render(<LibraryCard item={baseItem} />, { wrapper: wrapper() });

    const metaLine = screen.getByText(/example\.com/).parentElement!;

    expect(
      within(metaLine).getByRole("button", { name: /article actions/i }),
    ).toBeInTheDocument();
  });

  it("opens the menu without following the card link", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    render(
      <MemoryRouter initialEntries={["/"]}>
        <QueryClientProvider client={qc}>
          <Routes>
            <Route path="/" element={<LibraryCard item={baseItem} />} />
            <Route path="/library/7" element={<div>Reader opened</div>} />
          </Routes>
        </QueryClientProvider>
      </MemoryRouter>,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /article actions/i }),
    );

    expect(await screen.findByRole("menu")).toBeInTheDocument();
    expect(screen.queryByText("Reader opened")).not.toBeInTheDocument();
  });
});
