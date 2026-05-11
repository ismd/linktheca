import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { apiFetch } from "./client";
import { ApiError } from "./errors";
import { useAuthStore } from "@/features/auth/store";
import { writeRefreshToken, readRefreshToken, clearRefreshToken } from "@/features/auth/storage";

describe("apiFetch", () => {
  beforeEach(() => {
    server.use(
      http.get("/api/echo", () => HttpResponse.json({ ok: true })),
      http.get("/api/forbidden", () =>
        HttpResponse.json({ code: "forbidden", message: "Nope" }, { status: 403 }),
      ),
      http.get("/api/server-error", () =>
        HttpResponse.json({ code: "internal", message: "Boom" }, { status: 500 }),
      ),
      http.get("/api/no-json", () => new HttpResponse("plain text", { status: 502 })),
    );
  });

  it("prefixes /api and parses JSON", async () => {
    const data = await apiFetch<{ ok: boolean }>("/echo");
    expect(data.ok).toBe(true);
  });

  it("throws ApiError for 4xx with code+message from body", async () => {
    await expect(apiFetch("/forbidden")).rejects.toMatchObject({
      status: 403,
      code: "forbidden",
      message: "Nope",
    } satisfies Partial<ApiError>);
  });

  it("throws ApiError for 5xx", async () => {
    await expect(apiFetch("/server-error")).rejects.toBeInstanceOf(ApiError);
  });

  it("throws ApiError with synthetic code for non-JSON error", async () => {
    await expect(apiFetch("/no-json")).rejects.toMatchObject({
      status: 502,
      code: "http_error",
    });
  });

  describe("Authorization header", () => {
    beforeEach(() => {
      useAuthStore.getState().clearSession();
      server.use(
        http.get("/api/echo-auth", ({ request }) => {
          const auth = request.headers.get("Authorization");
          return HttpResponse.json({ auth });
        }),
      );
    });

    it("omits Authorization header when no token", async () => {
      const r = await apiFetch<{ auth: string | null }>("/echo-auth");
      expect(r.auth).toBeNull();
    });

    it("sends Bearer token when set in store", async () => {
      useAuthStore.getState().setSession("tok-xyz", {
        id: 1,
        email: "a@b.c",
        displayName: "A",
        isAdmin: false,
      });
      const r = await apiFetch<{ auth: string | null }>("/echo-auth");
      expect(r.auth).toBe("Bearer tok-xyz");
    });
  });
});

describe("apiFetch refresh-on-401", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    useAuthStore.setState({ status: "anonymous" });
    clearRefreshToken();
  });

  it("does NOT attempt refresh when no refresh token is stored", async () => {
    server.use(
      http.get("/api/protected", () =>
        HttpResponse.json({ code: "unauthorized", message: "no" }, { status: 401 }),
      ),
    );
    await expect(apiFetch("/protected")).rejects.toMatchObject({ status: 401 });
  });

  it("refreshes once on 401, updates session, retries original", async () => {
    writeRefreshToken("r-old");
    let refreshHits = 0;
    let protectedHits = 0;
    server.use(
      http.post("/api/auth/refresh", async ({ request }) => {
        refreshHits++;
        const body = (await request.json()) as { refresh_token: string };
        expect(body.refresh_token).toBe("r-old");
        return HttpResponse.json({
          user: {
            id: 1,
            email: "a@b.c",
            display_name: "A",
            is_admin: false,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
          tokens: { access_token: "a-new", refresh_token: "r-new" },
        });
      }),
      http.get("/api/protected", ({ request }) => {
        protectedHits++;
        const auth = request.headers.get("Authorization");
        if (auth === "Bearer a-new") return HttpResponse.json({ ok: true });
        return HttpResponse.json({ code: "unauthorized", message: "no" }, { status: 401 });
      }),
    );

    const data = await apiFetch<{ ok: boolean }>("/protected");
    expect(data.ok).toBe(true);
    expect(refreshHits).toBe(1);
    expect(protectedHits).toBe(2);
    expect(useAuthStore.getState().accessToken).toBe("a-new");
    expect(readRefreshToken()).toBe("r-new");
  });

  it("runs a single refresh for many parallel 401s", async () => {
    writeRefreshToken("r-old");
    let refreshHits = 0;
    server.use(
      http.post("/api/auth/refresh", () => {
        refreshHits++;
        return HttpResponse.json({
          user: {
            id: 1,
            email: "a@b.c",
            display_name: "A",
            is_admin: false,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          },
          tokens: { access_token: "a-new", refresh_token: "r-new" },
        });
      }),
      http.get("/api/a", ({ request }) =>
        request.headers.get("Authorization") === "Bearer a-new"
          ? HttpResponse.json({ which: "a" })
          : HttpResponse.json({ code: "u", message: "u" }, { status: 401 }),
      ),
      http.get("/api/b", ({ request }) =>
        request.headers.get("Authorization") === "Bearer a-new"
          ? HttpResponse.json({ which: "b" })
          : HttpResponse.json({ code: "u", message: "u" }, { status: 401 }),
      ),
    );

    const [a, b] = await Promise.all([
      apiFetch<{ which: string }>("/a"),
      apiFetch<{ which: string }>("/b"),
    ]);
    expect(a.which).toBe("a");
    expect(b.which).toBe("b");
    expect(refreshHits).toBe(1);
  });

  it("on refresh failure: clears session, removes refresh token, throws 401", async () => {
    writeRefreshToken("r-bad");
    server.use(
      http.post("/api/auth/refresh", () =>
        HttpResponse.json({ code: "invalid_refresh", message: "no" }, { status: 401 }),
      ),
      http.get("/api/protected", () =>
        HttpResponse.json({ code: "unauthorized", message: "no" }, { status: 401 }),
      ),
    );

    await expect(apiFetch("/protected")).rejects.toMatchObject({ status: 401 });
    expect(useAuthStore.getState().status).toBe("anonymous");
    expect(readRefreshToken()).toBeNull();
  });

  it("does NOT refresh on 401 from /auth/refresh itself", async () => {
    writeRefreshToken("r-x");
    let refreshHits = 0;
    server.use(
      http.post("/api/auth/refresh", () => {
        refreshHits++;
        return HttpResponse.json({ code: "x", message: "x" }, { status: 401 });
      }),
    );

    await expect(
      apiFetch("/auth/refresh", { method: "POST", body: JSON.stringify({ refresh_token: "r-x" }) }),
    ).rejects.toMatchObject({ status: 401 });
    expect(refreshHits).toBe(1);
  });
});
