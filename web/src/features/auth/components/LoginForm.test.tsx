import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { clearRefreshToken } from "@/features/auth/storage";
import { LoginForm } from "./LoginForm";

function setup() {
  const onSuccess = vi.fn();
  render(
    <MemoryRouter>
      <LoginForm onSuccess={onSuccess} />
    </MemoryRouter>,
  );
  return { onSuccess, user: userEvent.setup() };
}

describe("LoginForm", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    clearRefreshToken();
  });

  it("shows inline error when email is invalid", async () => {
    const { user } = setup();
    await user.type(screen.getByLabelText(/email/i), "not-an-email");
    await user.type(screen.getByLabelText(/password/i), "x");
    await user.click(screen.getByRole("button", { name: /sign in/i }));
    expect(await screen.findByText(/valid email/i)).toBeInTheDocument();
  });

  it("on success: calls onSuccess and stores session", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({
          user: {
            id: 1,
            email: "a@b.co",
            display_name: "A",
            is_admin: false,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
          tokens: { access_token: "a", refresh_token: "r" },
        }),
      ),
    );

    const { onSuccess, user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/password/i), "secret");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    await screen.findByRole("button", { name: /sign in/i }); // wait for re-render
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().status).toBe("authed");
  });

  it("on 401: shows 'Invalid email or password'", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json(
          { code: "invalid_credentials", message: "bad" },
          { status: 401 },
        ),
      ),
    );

    const { onSuccess, user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/password/i), "x");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/invalid email or password/i);
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("on 5xx: shows 'Service unavailable'", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({ code: "internal", message: "boom" }, { status: 500 }),
      ),
    );

    const { user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/password/i), "x");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/service unavailable/i);
  });
});
