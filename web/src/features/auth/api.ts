import { apiFetch } from "@/shared/api/client";
import { useAuthStore, type User } from "./store";
import { readRefreshToken, writeRefreshToken } from "./storage";
import { AuthResponseSchema, RawUserSchema, mapUser } from "./schemas";

export type LoginInput = { email: string; password: string };
export type RegisterInput = { email: string; password: string; displayName: string };

async function consumeAuthResponse(raw: unknown): Promise<User> {
  const parsed = AuthResponseSchema.parse(raw);
  const user = mapUser(parsed.user);
  writeRefreshToken(parsed.tokens.refresh_token);
  useAuthStore.getState().setSession(parsed.tokens.access_token, user);
  return user;
}

export async function login(input: LoginInput): Promise<User> {
  const raw = await apiFetch<unknown>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email: input.email, password: input.password }),
  });
  return consumeAuthResponse(raw);
}

export async function register(input: RegisterInput): Promise<User> {
  const raw = await apiFetch<unknown>("/auth/register", {
    method: "POST",
    body: JSON.stringify({
      email: input.email,
      password: input.password,
      display_name: input.displayName,
    }),
  });
  return consumeAuthResponse(raw);
}

export async function me(): Promise<User> {
  const raw = await apiFetch<unknown>("/auth/me");
  return mapUser(RawUserSchema.parse(raw));
}

export async function logout(refreshToken?: string): Promise<void> {
  const token = refreshToken ?? readRefreshToken();
  await apiFetch<void>("/auth/logout", {
    method: "POST",
    body: JSON.stringify({ refresh_token: token ?? "" }),
  });
}
