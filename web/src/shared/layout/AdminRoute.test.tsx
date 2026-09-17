import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router";
import { AdminRoute } from "./AdminRoute";
import { useAuthStore } from "@/features/auth/store";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="admin/sources" element={<p>Admin screen</p>} />
        </Route>
        <Route path="library" element={<p>Library screen</p>} />
        <Route path="login" element={<p>Login screen</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminRoute", () => {
  beforeEach(() => {
    useAuthStore.setState({ status: "anonymous", user: null });
  });

  it("lets an admin through", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: true,
    });
    renderAt("/admin/sources");
    expect(screen.getByText("Admin screen")).toBeInTheDocument();
  });

  it("sends a signed-in non-admin to the library", () => {
    useAuthStore.getState().setSession("t", {
      id: 2, email: "b@example.com", displayName: "B", isAdmin: false,
    });
    renderAt("/admin/sources");
    expect(screen.getByText("Library screen")).toBeInTheDocument();
  });

  it("sends an anonymous visitor to login", () => {
    renderAt("/admin/sources");
    expect(screen.getByText("Login screen")).toBeInTheDocument();
  });
});
