import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { apiFetch } from "./client";
import { ApiError } from "./errors";
import { useAuthStore } from "@/features/auth/store";

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
