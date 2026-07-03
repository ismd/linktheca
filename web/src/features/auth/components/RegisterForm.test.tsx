import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { clearRefreshToken } from "@/features/auth/storage";
import { RegisterForm } from "./RegisterForm";

function setup() {
  const onSuccess = vi.fn();
  const onRegistrationDisabled = vi.fn();
  render(
    <MemoryRouter>
      <RegisterForm onSuccess={onSuccess} onRegistrationDisabled={onRegistrationDisabled} />
    </MemoryRouter>,
  );
  return { onSuccess, onRegistrationDisabled, user: userEvent.setup() };
}

describe("RegisterForm", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    clearRefreshToken();
  });

  it("validates password length client-side", async () => {
    const { user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "short");
    await user.click(screen.getByRole("button", { name: /create account/i }));
    expect(
      await screen.findByText(/password must be at least 10 characters/i),
    ).toBeInTheDocument();
  });

  it("on success: calls onSuccess and stores session", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json(
          {
            user: {
              id: 1,
              email: "a@b.co",
              display_name: "Alice",
              is_admin: false,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
            tokens: { access_token: "a", refresh_token: "r" },
          },
          { status: 201 },
        ),
      ),
    );

    const { onSuccess, user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "p".repeat(10));
    await user.click(screen.getByRole("button", { name: /create account/i }));

    await screen.findByRole("button", { name: /create account/i });
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().status).toBe("authed");
  });

  it("on 409: shows 'email already exists'", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json(
          { error: "email_taken", message: "taken" },
          { status: 409 },
        ),
      ),
    );

    const { user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "p".repeat(10));
    await user.click(screen.getByRole("button", { name: /create account/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/email already exists/i);
  });

  it("on 403: calls onRegistrationDisabled", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json(
          { error: "registration_disabled", message: "off" },
          { status: 403 },
        ),
      ),
    );

    const { onRegistrationDisabled, user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.co");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "p".repeat(10));
    await user.click(screen.getByRole("button", { name: /create account/i }));

    await vi.waitFor(() => expect(onRegistrationDisabled).toHaveBeenCalledTimes(1));
  });
});
