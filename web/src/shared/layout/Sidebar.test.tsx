import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { Sidebar } from "./Sidebar";

describe("Sidebar", () => {
  function renderWithRouter() {
    return render(
      <MemoryRouter>
        <Sidebar />
      </MemoryRouter>,
    );
  }

  it("renders the masthead", () => {
    renderWithRouter();
    expect(screen.getByText("Linktheca")).toBeInTheDocument();
  });

  it("renders Library and Settings as enabled nav links", () => {
    renderWithRouter();
    expect(screen.getByRole("link", { name: /library/i })).toHaveAttribute("href", "/library");
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "/settings");
  });

  it("renders Radar as disabled (no link)", () => {
    renderWithRouter();
    const radar = screen.getByText("Radar");
    expect(radar.closest("a")).toBeNull();
    expect(screen.getByText(/soon/i)).toBeInTheDocument();
  });
});
