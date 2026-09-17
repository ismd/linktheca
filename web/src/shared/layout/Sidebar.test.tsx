import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { Sidebar } from "./Sidebar";
import { useAuthStore } from "@/features/auth/store";

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

  it("renders Library, Radar, and Settings as enabled nav links", () => {
    useAuthStore.setState({ status: "anonymous", user: null });
    renderWithRouter();
    expect(screen.getByRole("link", { name: /library/i })).toHaveAttribute("href", "/library");
    expect(screen.getByRole("link", { name: /radar/i })).toHaveAttribute("href", "/radar");
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "/settings");
  });

  it("hides Admin from a non-admin", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: false,
    });
    renderWithRouter();
    expect(screen.queryByRole("link", { name: /admin/i })).not.toBeInTheDocument();
  });

  it("shows Admin to an admin", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: true,
    });
    renderWithRouter();
    expect(screen.getByRole("link", { name: /admin/i })).toHaveAttribute("href", "/admin/sources");
  });
});
