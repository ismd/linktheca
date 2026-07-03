import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "./store";
import { writeRefreshToken, readRefreshToken, clearRefreshToken } from "./storage";
import { login, register, me, logout } from "./api";
import { ApiError } from "@/shared/api/errors";

const userPayload = {
  id: 7,
  email: "a@b.c",
  display_name: "A",
  is_admin: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

describe("auth api", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    clearRefreshToken();
  });

  it("login: stores session and refresh token on success", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json({
          user: userPayload,
          tokens: { access_token: "a1", refresh_token: "r1" },
        }),
      ),
    );

    await login({ email: "a@b.c", password: "secret" });

    const s = useAuthStore.getState();
    expect(s.status).toBe("authed");
    expect(s.accessToken).toBe("a1");
    expect(s.user?.displayName).toBe("A");
    expect(readRefreshToken()).toBe("r1");
  });

  it("login: propagates ApiError on 401 without touching state", async () => {
    server.use(
      http.post("/api/auth/login", () =>
        HttpResponse.json(
          { error: "invalid_credentials", message: "bad creds" },
          { status: 401 },
        ),
      ),
    );

    await expect(login({ email: "a@b.c", password: "x" })).rejects.toBeInstanceOf(ApiError);
    expect(useAuthStore.getState().status).toBe("anonymous");
    expect(readRefreshToken()).toBeNull();
  });

  it("register: stores session and refresh token on success", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json(
          {
            user: userPayload,
            tokens: { access_token: "a2", refresh_token: "r2" },
          },
          { status: 201 },
        ),
      ),
    );

    await register({ email: "a@b.c", password: "p".repeat(10), displayName: "A" });

    expect(useAuthStore.getState().accessToken).toBe("a2");
    expect(readRefreshToken()).toBe("r2");
  });

  it("me: returns mapped user", async () => {
    server.use(
      http.get("/api/auth/me", () => HttpResponse.json(userPayload)),
    );

    const u = await me();
    expect(u.displayName).toBe("A");
    expect(u.isAdmin).toBe(false);
  });

  it("logout: sends refresh token, returns void", async () => {
    writeRefreshToken("r-keep");
    let captured: unknown;
    server.use(
      http.post("/api/auth/logout", async ({ request }) => {
        captured = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await logout();
    expect(captured).toEqual({ refresh_token: "r-keep" });
  });
});
