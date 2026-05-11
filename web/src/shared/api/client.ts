import { ApiError } from "./errors";
import { useAuthStore } from "@/features/auth/store";
import {
  readRefreshToken,
  writeRefreshToken,
  clearRefreshToken,
} from "@/features/auth/storage";
import { AuthResponseSchema, mapUser } from "@/features/auth/schemas";

const API_BASE = "/api";
const REFRESH_PATH = "/auth/refresh";

let refreshPromise: Promise<string> | null = null;

async function performRefresh(): Promise<string> {
  const refreshToken = readRefreshToken();
  if (!refreshToken) {
    throw new ApiError(401, "no_refresh_token", "no refresh token");
  }
  const res = await fetch(`${API_BASE}${REFRESH_PATH}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, "refresh_failed", "refresh failed");
  }
  const body = (await res.json()) as unknown;
  const parsed = AuthResponseSchema.parse(body);
  writeRefreshToken(parsed.tokens.refresh_token);
  useAuthStore.getState().setSession(parsed.tokens.access_token, mapUser(parsed.user));
  return parsed.tokens.access_token;
}

function refreshOnce(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

type Options = { _retry?: boolean };

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  opts?: Options,
): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type") && init?.body) {
    headers.set("Content-Type", "application/json");
  }
  const token = useAuthStore.getState().accessToken;
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(`${API_BASE}${path}`, { ...init, headers });

  if (res.ok) {
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  if (
    res.status === 401 &&
    !opts?._retry &&
    path !== REFRESH_PATH &&
    readRefreshToken()
  ) {
    try {
      await refreshOnce();
    } catch {
      clearRefreshToken();
      useAuthStore.getState().clearSession();
      throw new ApiError(401, "session_expired", "session expired");
    }
    return apiFetch<T>(path, init, { _retry: true });
  }

  let code = "http_error";
  let message = res.statusText || "Request failed";
  let details: unknown;

  const ct = res.headers.get("content-type") ?? "";
  if (ct.includes("application/json")) {
    try {
      const body = (await res.json()) as {
        code?: string;
        message?: string;
        details?: unknown;
      };
      if (typeof body.code === "string") code = body.code;
      if (typeof body.message === "string") message = body.message;
      details = body.details;
    } catch {
      // fall through
    }
  }

  throw new ApiError(res.status, code, message, details);
}
