import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "./store";
import { writeRefreshToken, clearRefreshToken, readRefreshToken } from "./storage";
import { useBootstrap } from "./use-bootstrap";

describe("useBootstrap", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    useAuthStore.setState({ status: "bootstrapping" });
    clearRefreshToken();
  });

  it("becomes anonymous immediately when no refresh token is stored", async () => {
    renderHook(() => useBootstrap());
    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("anonymous");
    });
  });

  it("becomes authed from refresh alone, even if /auth/me would answer without a token", async () => {
    // The browser used to replay a stored /auth/me 200 to the tokenless
    // bootstrap request, so it never refreshed and stayed on the spinner
    writeRefreshToken("r-good");
    let meHits = 0;
    server.use(
      http.post("/api/auth/refresh", () =>
        HttpResponse.json({
          user: {
            id: 1,
            email: "a@b.c",
            display_name: "A",
            is_admin: false,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
          tokens: { access_token: "a-new", refresh_token: "r-new" },
        }),
      ),
      http.get("/api/auth/me", () => {
        meHits++;
        return HttpResponse.json({
          id: 1,
          email: "a@b.c",
          display_name: "A",
          is_admin: false,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        });
      }),
    );

    renderHook(() => useBootstrap());

    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("authed");
    });
    expect(useAuthStore.getState().accessToken).toBe("a-new");
    expect(useAuthStore.getState().user?.displayName).toBe("A");
    expect(readRefreshToken()).toBe("r-new");
    expect(meHits).toBe(0);
  });

  it("becomes anonymous and clears refresh token when refresh fails", async () => {
    writeRefreshToken("r-bad");
    server.use(
      http.post("/api/auth/refresh", () =>
        HttpResponse.json({ error: "x", message: "x" }, { status: 401 }),
      ),
    );

    renderHook(() => useBootstrap());

    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("anonymous");
    });
    expect(readRefreshToken()).toBeNull();
  });

  it("becomes anonymous but keeps refresh token when the server is unreachable", async () => {
    writeRefreshToken("r-keep");
    server.use(http.post("/api/auth/refresh", () => HttpResponse.error()));

    renderHook(() => useBootstrap());

    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("anonymous");
    });
    expect(readRefreshToken()).toBe("r-keep");
  });
});
