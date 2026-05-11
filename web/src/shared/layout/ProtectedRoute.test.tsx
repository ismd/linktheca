import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { ProtectedRoute } from "./ProtectedRoute";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route path="/library" element={<div>library content</div>} />
        </Route>
        <Route path="/login" element={<div>login page</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ProtectedRoute", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
  });

  it("renders FullPageSpinner while bootstrapping", () => {
    useAuthStore.setState({ status: "bootstrapping" });
    renderAt("/library");
    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByText("library content")).not.toBeInTheDocument();
  });

  it("redirects anonymous user to /login", () => {
    useAuthStore.setState({ status: "anonymous" });
    renderAt("/library");
    expect(screen.getByText("login page")).toBeInTheDocument();
    expect(screen.queryByText("library content")).not.toBeInTheDocument();
  });

  it("renders Outlet for authed user", () => {
    useAuthStore.getState().setSession("t", {
      id: 1,
      email: "a@b.c",
      displayName: "A",
      isAdmin: false,
    });
    renderAt("/library");
    expect(screen.getByText("library content")).toBeInTheDocument();
  });
});
