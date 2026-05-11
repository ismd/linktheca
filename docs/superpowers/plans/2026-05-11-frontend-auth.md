# Frontend Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Поверх готового frontend foundation поднять полноценный auth-flow: Zod-схемы и API-функции, refresh-on-401 с in-flight singleton, bootstrap при загрузке, `<ProtectedRoute>`, Login/Register формы (React Hook Form + Zod), Logout через UserMenu в Topbar, sonner-тосты. После плана пользователь может зарегистрироваться, залогиниться, переключаться между сессиями и видеть `/library` (контента ещё нет — это следующий план).

**Architecture:** Refresh-token живёт в `localStorage` (`linktheca.refresh`), access-token — только в Zustand store. `apiFetch` сам обрабатывает 401: одиночный in-flight Promise `POST /auth/refresh` запускается из первого 401, остальные 401-запросы ждут тот же Promise, после успеха оригиналы ретраятся ровно один раз. При провале refresh — `clearSession()`, и `<ProtectedRoute>` редиректит на `/login` с `from`-location в state. Bootstrap при mount → `status='bootstrapping'` пока не отработает refresh из storage; `<ProtectedRoute>` рендерит `<FullPageSpinner>` пока bootstrapping. Login/Register — React Hook Form + Zod, серверные ошибки маппятся в top-of-form alert или inline-field-error.

**Tech Stack:** zod, react-hook-form, @hookform/resolvers, sonner; уже стоящие React Router v7, TanStack Query v5, Zustand, Radix Dialog/Slot/Label, shadcn/ui copies, lucide-react, Vitest + RTL + MSW.

**Spec:** `docs/superpowers/specs/2026-05-08-frontend-foundation-library-design.md` секции 3–4.

---

## Файловая структура (создаётся/меняется этим планом)

```
web/
  package.json                                      # +zod, +react-hook-form, +@hookform/resolvers, +sonner
  src/
    App.tsx                                         # mount Toaster + use useBootstrap + ProtectedRoute wrapper
    main.tsx                                        # (no change)
    routes/
      _public.tsx                                   # masthead + .rule-double визуал
      login.tsx                                     # renders <LoginForm/>
      register.tsx                                  # renders <RegisterForm/> или <RegistrationDisabled/>
    features/
      auth/
        api.ts                                      # NEW — login/register/refresh/logout/me functions
        api.test.ts                                 # NEW
        schemas.ts                                  # NEW — Zod schemas for auth responses
        storage.ts                                  # NEW — localStorage helpers for refresh token
        storage.test.ts                             # NEW
        use-bootstrap.ts                            # NEW — hook for App.tsx
        use-bootstrap.test.tsx                      # NEW
        use-logout.ts                               # NEW — useMutation that wraps POST /auth/logout
        components/
          LoginForm.tsx                             # NEW
          LoginForm.test.tsx                        # NEW
          RegisterForm.tsx                          # NEW
          RegisterForm.test.tsx                     # NEW
          RegistrationDisabled.tsx                  # NEW — full-page «registration is closed» сообщение
          UserMenu.tsx                              # NEW — initials → dropdown → logout
          PublicMasthead.tsx                        # NEW — «Linktheca» italic header для public layout
    shared/
      api/
        client.ts                                   # ADD refresh-on-401 singleton + retry-once
        client.test.ts                              # ADD refresh tests
      layout/
        FullPageSpinner.tsx                         # NEW
        ProtectedRoute.tsx                          # NEW
        ProtectedRoute.test.tsx                     # NEW
        Topbar.tsx                                  # replace placeholder user square with <UserMenu/>
      ui/
        dropdown-menu.tsx                           # NEW shadcn copy (Radix DropdownMenu wrapper)
        sonner.tsx                                  # NEW — <Toaster/> wrapper
```

---

## Группа 1 — Deps + schemas + API + storage

### Task 1: Установить зависимости

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`

- [x] **Step 1: Install packages**

Run from `/home/ismd/coding/linktheca/web`:
```bash
npm install zod react-hook-form @hookform/resolvers sonner @radix-ui/react-dropdown-menu
```

Expected: `package.json` gets new entries in `dependencies`. Latest stable versions at time of writing: `zod@^3.23`, `react-hook-form@^7.55`, `@hookform/resolvers@^4.0`, `sonner@^1.7`, `@radix-ui/react-dropdown-menu@^2.1`. Don't pin patch — use caret as elsewhere.

- [x] **Step 2: Verify install**

Run: `npm run typecheck`
Expected: PASS (no usage yet, just installs).

Run: `npm run test`
Expected: PASS (existing tests unchanged).

- [x] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "deps(web): add zod, react-hook-form, sonner, radix dropdown-menu"
```

---

### Task 2: Zod schemas для auth-ответов

**Files:**
- Create: `web/src/features/auth/schemas.ts`

Backend serializes User in snake_case (`display_name`, `is_admin`, `created_at`, `updated_at`); TokenPair as `access_token`/`refresh_token`; AuthResponse as `{ user, tokens }`. На фронте мы используем camelCase для `User` (`displayName`, `isAdmin`). Zod-схемы парсят raw shape, маппинг к camelCase делаем в `mapUser()` ниже.

- [x] **Step 1: Write the file**

```ts
// web/src/features/auth/schemas.ts
import { z } from "zod";
import type { User } from "./store";

export const RawUserSchema = z.object({
  id: z.number().int(),
  email: z.string(),
  display_name: z.string(),
  is_admin: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const TokenPairSchema = z.object({
  access_token: z.string().min(1),
  refresh_token: z.string().min(1),
});

export const AuthResponseSchema = z.object({
  user: RawUserSchema,
  tokens: TokenPairSchema,
});

export type RawUser = z.infer<typeof RawUserSchema>;
export type TokenPair = z.infer<typeof TokenPairSchema>;
export type AuthResponse = z.infer<typeof AuthResponseSchema>;

export function mapUser(raw: RawUser): User {
  return {
    id: raw.id,
    email: raw.email,
    displayName: raw.display_name,
    isAdmin: raw.is_admin,
  };
}
```

- [x] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/auth/schemas.ts
git commit -m "feat(web/auth): add Zod schemas for auth responses and User mapper"
```

---

### Task 3: Refresh-token storage helpers

**Files:**
- Create: `web/src/features/auth/storage.ts`
- Test: `web/src/features/auth/storage.test.ts`

Зачем отдельный модуль: спрятать ключ `linktheca.refresh` за тремя функциями (`read`, `write`, `clear`), чтобы тесты могли мокать одно место и чтобы доступ к localStorage не разрастался.

- [x] **Step 1: Write the failing test**

```ts
// web/src/features/auth/storage.test.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readRefreshToken, writeRefreshToken, clearRefreshToken } from "./storage";

describe("refresh token storage", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("returns null when no token is stored", () => {
    expect(readRefreshToken()).toBeNull();
  });

  it("write then read returns the same value", () => {
    writeRefreshToken("abc");
    expect(readRefreshToken()).toBe("abc");
  });

  it("clear removes the token", () => {
    writeRefreshToken("abc");
    clearRefreshToken();
    expect(readRefreshToken()).toBeNull();
  });

  it("returns null and swallows error when localStorage throws", () => {
    const original = Storage.prototype.getItem;
    Storage.prototype.getItem = () => {
      throw new Error("disabled");
    };
    try {
      expect(readRefreshToken()).toBeNull();
    } finally {
      Storage.prototype.getItem = original;
    }
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test -- storage`
Expected: FAIL with "Failed to resolve import ./storage".

- [x] **Step 3: Implement**

```ts
// web/src/features/auth/storage.ts
const KEY = "linktheca.refresh";

export function readRefreshToken(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function writeRefreshToken(token: string): void {
  try {
    localStorage.setItem(KEY, token);
  } catch {
    // ignore — storage may be disabled in private mode
  }
}

export function clearRefreshToken(): void {
  try {
    localStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}
```

- [x] **Step 4: Run tests**

Run: `npm run test -- storage`
Expected: PASS (4 tests).

- [x] **Step 5: Commit**

```bash
git add web/src/features/auth/storage.ts web/src/features/auth/storage.test.ts
git commit -m "feat(web/auth): add refresh token localStorage helpers"
```

---

### Task 4: Auth API functions

**Files:**
- Create: `web/src/features/auth/api.ts`
- Test: `web/src/features/auth/api.test.ts`

Этот модуль НЕ занимается refresh-логикой — он только зовёт `apiFetch` и парсит ответы. Refresh-on-401 живёт внутри `apiFetch` (Task 6). Login/Register **раньше bootstrap-а делают side-effects** (`setSession` + `writeRefreshToken`), потому что апп должен сразу стать authed. Logout наоборот — side-effects делает консамер (`use-logout`), потому что нужен `queryClient.clear()`.

- [x] **Step 1: Write the failing test**

```ts
// web/src/features/auth/api.test.ts
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
          { code: "invalid_credentials", message: "bad creds" },
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
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test -- features/auth/api`
Expected: FAIL with "Failed to resolve import ./api".

- [x] **Step 3: Implement**

```ts
// web/src/features/auth/api.ts
import { apiFetch } from "@/shared/api/client";
import { useAuthStore, type User } from "./store";
import { writeRefreshToken } from "./storage";
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
  const token = refreshToken ?? (await import("./storage")).readRefreshToken();
  await apiFetch<void>("/auth/logout", {
    method: "POST",
    body: JSON.stringify({ refresh_token: token ?? "" }),
  });
}
```

Replace dynamic import in `logout` with a normal import to keep the bundle simple:

```ts
import { readRefreshToken, writeRefreshToken } from "./storage";
// ... and inside logout:
export async function logout(refreshToken?: string): Promise<void> {
  const token = refreshToken ?? readRefreshToken();
  await apiFetch<void>("/auth/logout", {
    method: "POST",
    body: JSON.stringify({ refresh_token: token ?? "" }),
  });
}
```

Final file:

```ts
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
```

- [x] **Step 4: Run tests**

Run: `npm run test -- features/auth/api`
Expected: PASS (5 tests).

Also: `npm run typecheck` → PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/auth/api.ts web/src/features/auth/api.test.ts
git commit -m "feat(web/auth): add login/register/refresh-aware me/logout API functions"
```

---

## Группа 2 — Refresh-on-401 в apiFetch

### Task 5: Refresh-on-401 singleton + retry-once

**Files:**
- Modify: `web/src/shared/api/client.ts`
- Modify: `web/src/shared/api/client.test.ts`

Спецификация:
1. 401 + есть refresh-token + это не `/auth/refresh` сам → запустить refresh (singleton Promise, один in-flight на все параллельные 401), после успеха ретраить **ровно один раз**.
2. Refresh-успех → `setSession(newAccess, mappedUser)` + `writeRefreshToken(newRefresh)`.
3. Refresh-провал (любой статус, network error, parse error) → `clearRefreshToken()`, `clearSession()`, прокинуть ApiError(401) дальше. ProtectedRoute среагирует.
4. Ретрай оригинального запроса использует **новый** access-token из store (apiFetch читает токен в начале каждого вызова — уже так).
5. Запросы с `_retry: true` НЕ инициируют второй refresh — если снова 401, бросаем ApiError.

- [x] **Step 1: Write failing tests (extend existing file)**

Add to `web/src/shared/api/client.test.ts`, в конец файла:

```ts
import { writeRefreshToken, readRefreshToken, clearRefreshToken } from "@/features/auth/storage";

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
```

- [x] **Step 2: Run tests to verify they fail**

Run: `npm run test -- shared/api/client`
Expected: FAIL — new describe-block fails (refresh-related behavior not yet implemented).

- [x] **Step 3: Implement refresh-on-401**

Replace `web/src/shared/api/client.ts` entirely:

```ts
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
```

- [x] **Step 4: Run tests**

Run: `npm run test -- shared/api/client`
Expected: PASS (all existing + 5 new refresh tests).

Run: `npm run typecheck`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/shared/api/client.ts web/src/shared/api/client.test.ts
git commit -m "feat(web/api): refresh-on-401 with singleton in-flight Promise and retry-once"
```

---

## Группа 3 — Bootstrap + ProtectedRoute

### Task 6: FullPageSpinner

**Files:**
- Create: `web/src/shared/layout/FullPageSpinner.tsx`

Тонкий компонент, без логики. Визуально — paper-surface на весь экран, центрированная подпись «Linktheca» Cormorant italic + ниже маленький subtle pulse-dot ряд. Без зависимостей от lucide-spinner (статичные точки выглядят editorial-нее).

- [x] **Step 1: Implement**

```tsx
// web/src/shared/layout/FullPageSpinner.tsx
export function FullPageSpinner() {
  return (
    <div
      role="status"
      aria-live="polite"
      className="paper-surface fixed inset-0 flex flex-col items-center justify-center gap-6"
    >
      <p className="font-display italic text-4xl text-ink leading-none">Linktheca</p>
      <div className="flex gap-2">
        <span className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse" />
        <span
          className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse"
          style={{ animationDelay: "120ms" }}
        />
        <span
          className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse"
          style={{ animationDelay: "240ms" }}
        />
      </div>
      <span className="sr-only">Loading</span>
    </div>
  );
}
```

- [x] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/shared/layout/FullPageSpinner.tsx
git commit -m "feat(web): add FullPageSpinner for bootstrapping state"
```

---

### Task 7: useBootstrap hook

**Files:**
- Create: `web/src/features/auth/use-bootstrap.ts`
- Test: `web/src/features/auth/use-bootstrap.test.tsx`

Хук вызывается **один раз** при mount в `App.tsx`. Поведение:
- При mount: если в localStorage нет refresh-токена → `status='anonymous'`, выходим.
- Если refresh-токен есть → пытаемся `POST /auth/refresh`. На успех `setSession` (уже делает `performRefresh` внутри apiFetch — но тут мы зовём низкоуровневый запрос). На провал → `clearRefreshToken()`, `clearSession()`.

Тонкость: можно ли переиспользовать `apiFetch('/auth/me')`? Да: оно само словит 401, дёрнет refresh, ретрайнёт `/auth/me`. После успеха store будет authed. На провал — store anonymous. Этот путь короче.

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/auth/use-bootstrap.test.tsx
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

  it("becomes authed after successful refresh + me", async () => {
    writeRefreshToken("r-good");
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
      http.get("/api/auth/me", ({ request }) => {
        if (request.headers.get("Authorization") === "Bearer a-new") {
          return HttpResponse.json({
            id: 1,
            email: "a@b.c",
            display_name: "A",
            is_admin: false,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: "2026-01-01T00:00:00Z",
          });
        }
        return HttpResponse.json({ code: "u", message: "u" }, { status: 401 });
      }),
    );

    renderHook(() => useBootstrap());

    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("authed");
    });
    expect(useAuthStore.getState().accessToken).toBe("a-new");
  });

  it("becomes anonymous and clears refresh token when refresh fails", async () => {
    writeRefreshToken("r-bad");
    server.use(
      http.post("/api/auth/refresh", () =>
        HttpResponse.json({ code: "x", message: "x" }, { status: 401 }),
      ),
      http.get("/api/auth/me", () =>
        HttpResponse.json({ code: "u", message: "u" }, { status: 401 }),
      ),
    );

    renderHook(() => useBootstrap());

    await waitFor(() => {
      expect(useAuthStore.getState().status).toBe("anonymous");
    });
    expect(readRefreshToken()).toBeNull();
  });
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `npm run test -- use-bootstrap`
Expected: FAIL with "Failed to resolve import ./use-bootstrap".

- [x] **Step 3: Implement**

```ts
// web/src/features/auth/use-bootstrap.ts
import { useEffect, useRef } from "react";
import { useAuthStore } from "./store";
import { readRefreshToken } from "./storage";
import { me } from "./api";

export function useBootstrap(): void {
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;

    const refreshToken = readRefreshToken();
    if (!refreshToken) {
      useAuthStore.getState().clearSession();
      return;
    }

    (async () => {
      try {
        await me(); // апи сам зовёт /auth/refresh при 401 и обновит store
      } catch {
        useAuthStore.getState().clearSession();
      }
    })();
  }, []);
}
```

- [x] **Step 4: Run tests**

Run: `npm run test -- use-bootstrap`
Expected: PASS (3 tests).

- [x] **Step 5: Commit**

```bash
git add web/src/features/auth/use-bootstrap.ts web/src/features/auth/use-bootstrap.test.tsx
git commit -m "feat(web/auth): add useBootstrap hook to restore session from refresh token"
```

---

### Task 8: ProtectedRoute component

**Files:**
- Create: `web/src/shared/layout/ProtectedRoute.tsx`
- Test: `web/src/shared/layout/ProtectedRoute.test.tsx`

Поведение:
- `status === 'bootstrapping'` → `<FullPageSpinner />`.
- `status === 'anonymous'` → `<Navigate to="/login" state={{ from: location }} replace />`.
- `status === 'authed'` → `<Outlet />`.

`from`-location сохраняется в state, чтобы `/login` мог редиректить обратно после успеха.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/shared/layout/ProtectedRoute.test.tsx
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- ProtectedRoute`
Expected: FAIL with import error.

- [ ] **Step 3: Implement**

```tsx
// web/src/shared/layout/ProtectedRoute.tsx
import { Navigate, Outlet, useLocation } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { FullPageSpinner } from "./FullPageSpinner";

export function ProtectedRoute() {
  const status = useAuthStore((s) => s.status);
  const location = useLocation();

  if (status === "bootstrapping") return <FullPageSpinner />;
  if (status === "anonymous") {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return <Outlet />;
}
```

- [ ] **Step 4: Run tests**

Run: `npm run test -- ProtectedRoute`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/shared/layout/ProtectedRoute.tsx web/src/shared/layout/ProtectedRoute.test.tsx
git commit -m "feat(web): add ProtectedRoute with bootstrapping/anonymous/authed branches"
```

---

### Task 9: Wire useBootstrap + ProtectedRoute into App

**Files:**
- Modify: `web/src/App.tsx`

В router-tree оборачиваем `AppLayout`-ветку в `ProtectedRoute`. `useBootstrap` зовём из тонкого компонента-обёртки внутри `<QueryClientProvider>`.

- [ ] **Step 1: Update App.tsx**

```tsx
// web/src/App.tsx
import { createBrowserRouter } from "react-router";
import { RouterProvider } from "react-router/dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/shared/api/query-client";
import { useBootstrap } from "@/features/auth/use-bootstrap";
import { ProtectedRoute } from "@/shared/layout/ProtectedRoute";
import RootLayout from "./routes/__root";
import PublicLayout from "./routes/_public";
import AppLayout from "./routes/__app";
import IndexRoute from "./routes/index";
import LoginRoute from "./routes/login";
import RegisterRoute from "./routes/register";
import LibraryListRoute from "./routes/library._index";
import LibraryItemRoute from "./routes/library.$id";
import SettingsRoute from "./routes/settings";
import NotFoundRoute from "./routes/not-found";

const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <IndexRoute /> },
      {
        element: <PublicLayout />,
        children: [
          { path: "login", element: <LoginRoute /> },
          { path: "register", element: <RegisterRoute /> },
        ],
      },
      {
        element: <ProtectedRoute />,
        children: [
          {
            element: <AppLayout />,
            children: [
              { path: "library", element: <LibraryListRoute /> },
              { path: "library/:id", element: <LibraryItemRoute /> },
              { path: "settings", element: <SettingsRoute /> },
            ],
          },
        ],
      },
      { path: "*", element: <NotFoundRoute /> },
    ],
  },
]);

function BootstrapGate({ children }: { children: React.ReactNode }) {
  useBootstrap();
  return <>{children}</>;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BootstrapGate>
        <RouterProvider router={router} />
      </BootstrapGate>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 2: Run tests + typecheck**

Run: `npm run test`
Expected: PASS (all existing).

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Smoke-test dev server**

Run in one terminal: `npm run dev`
Open `http://localhost:5173/`. With no backend running, Vite proxy will fail, but the page should render FullPageSpinner briefly then settle on `/login` (because `/auth/me` fails → bootstrap clears session → ProtectedRoute redirects). The current `/login` stub is still text; визуал в Task 11.

If you don't want to spin up the backend, you can also temporarily delete the refresh token from `localStorage` in the browser console — bootstrap goes straight to anonymous and redirects to /login. Confirm the spinner appears for at least a frame before the redirect.

Stop the dev server (`Ctrl+C`).

- [ ] **Step 4: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(web): wire bootstrap and ProtectedRoute into router"
```

---

## Группа 4 — Public layout + Login

### Task 10: PublicMasthead component

**Files:**
- Create: `web/src/features/auth/components/PublicMasthead.tsx`

«Linktheca» Cormorant italic + декоративный `.rule-double` ниже. Используем в `_public.tsx` (Login/Register).

- [ ] **Step 1: Implement**

```tsx
// web/src/features/auth/components/PublicMasthead.tsx
export function PublicMasthead() {
  return (
    <div className="text-center mb-8">
      <p className="font-display italic text-5xl text-ink leading-none">Linktheca</p>
      <p className="label-sc mt-3 text-muted">A private archive</p>
      <div className="rule-double mt-6" />
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/auth/components/PublicMasthead.tsx
git commit -m "feat(web/auth): add PublicMasthead for login/register pages"
```

---

### Task 11: Update _public layout with masthead

**Files:**
- Modify: `web/src/routes/_public.tsx`

- [ ] **Step 1: Edit**

```tsx
// web/src/routes/_public.tsx
import { Outlet } from "react-router";
import { PublicMasthead } from "@/features/auth/components/PublicMasthead";

export default function PublicLayout() {
  return (
    <div className="paper-surface min-h-screen flex items-center justify-center p-8">
      <div className="w-full max-w-md">
        <PublicMasthead />
        <Outlet />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/_public.tsx
git commit -m "feat(web): mount PublicMasthead in _public layout"
```

---

### Task 12: LoginForm component (React Hook Form + Zod)

**Files:**
- Create: `web/src/features/auth/components/LoginForm.tsx`
- Test: `web/src/features/auth/components/LoginForm.test.tsx`

Поведение:
- Поля: `email` (валидный email), `password` (≥1 char — длину проверит бэкенд).
- Inline-error для каждого поля через `aria-describedby`.
- Top-of-form alert:
  - `401 invalid_credentials` → «Invalid email or password».
  - `5xx` → «Service unavailable. Please try again.».
  - Прочее → берём `error.message` как fallback.
- Submit-кнопка disabled пока `formState.isSubmitting`.
- На success зовёт `onSuccess()`-callback (роут сам делает navigate).

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/features/auth/components/LoginForm.test.tsx
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { clearRefreshToken } from "@/features/auth/storage";
import { LoginForm } from "./LoginForm";

function setup() {
  const onSuccess = vi.fn();
  render(<LoginForm onSuccess={onSuccess} />);
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
            email: "a@b.c",
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
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
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
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
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
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
    await user.type(screen.getByLabelText(/password/i), "x");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/service unavailable/i);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- LoginForm`
Expected: FAIL with import error.

- [ ] **Step 3: Implement**

```tsx
// web/src/features/auth/components/LoginForm.tsx
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { login } from "@/features/auth/api";
import { ApiError } from "@/shared/api/errors";

const schema = z.object({
  email: z.string().email("Please enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "Invalid email or password";
    if (err.status >= 500) return "Service unavailable. Please try again.";
    return err.message || "Something went wrong";
  }
  return "Something went wrong";
}

type Props = {
  onSuccess: () => void;
};

export function LoginForm({ onSuccess }: Props) {
  const [topError, setTopError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    try {
      await login(values);
      onSuccess();
    } catch (err) {
      setTopError(mapError(err));
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-4 py-3 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="email" className="label-sc text-ink-3">
          Email
        </Label>
        <Input
          id="email"
          type="email"
          autoComplete="email"
          aria-invalid={errors.email ? "true" : "false"}
          aria-describedby={errors.email ? "email-error" : undefined}
          {...register("email")}
        />
        {errors.email && (
          <p id="email-error" className="text-sm text-vermillion-dark">
            {errors.email.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="password" className="label-sc text-ink-3">
          Password
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete="current-password"
          aria-invalid={errors.password ? "true" : "false"}
          aria-describedby={errors.password ? "password-error" : undefined}
          {...register("password")}
        />
        {errors.password && (
          <p id="password-error" className="text-sm text-vermillion-dark">
            {errors.password.message}
          </p>
        )}
      </div>

      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Signing in…" : "Sign in"}
      </Button>

      <p className="label-sc text-center text-muted">
        <Link to="/register" className="hover:text-ink-3">
          Create account →
        </Link>
      </p>
    </form>
  );
}
```

- [ ] **Step 4: Run tests**

Run: `npm run test -- LoginForm`
Expected: PASS (4 tests).

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/auth/components/LoginForm.tsx web/src/features/auth/components/LoginForm.test.tsx
git commit -m "feat(web/auth): add LoginForm with RHF+Zod validation and error mapping"
```

---

### Task 13: Wire LoginForm into /login route + redirect to from-location

**Files:**
- Modify: `web/src/routes/login.tsx`

- [ ] **Step 1: Edit**

```tsx
// web/src/routes/login.tsx
import { useLocation, useNavigate } from "react-router";
import { LoginForm } from "@/features/auth/components/LoginForm";

type LocationState = { from?: { pathname?: string } } | null;

export default function LoginRoute() {
  const navigate = useNavigate();
  const location = useLocation();
  const state = location.state as LocationState;
  const from = state?.from?.pathname ?? "/library";

  return (
    <LoginForm
      onSuccess={() => {
        navigate(from, { replace: true });
      }}
    />
  );
}
```

- [ ] **Step 2: Run tests + typecheck**

Run: `npm run test` and `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/login.tsx
git commit -m "feat(web): wire LoginForm into /login with from-location redirect"
```

---

## Группа 5 — Register

### Task 14: RegistrationDisabled component

**Files:**
- Create: `web/src/features/auth/components/RegistrationDisabled.tsx`

Полностраничное сообщение «New accounts are disabled on this instance.», отображается, когда бэкенд возвращает 403 `registration_disabled` на `POST /auth/register`. Сообщение появляется ВНУТРИ public layout (masthead уже сверху), не вместо него.

- [ ] **Step 1: Implement**

```tsx
// web/src/features/auth/components/RegistrationDisabled.tsx
import { Link } from "react-router";

export function RegistrationDisabled() {
  return (
    <div className="text-center">
      <p className="font-display text-2xl text-ink-2 mb-3">Registration is closed</p>
      <p className="text-sm text-ink-3 mb-6">
        New accounts are disabled on this instance.
      </p>
      <p className="label-sc text-muted">
        <Link to="/login" className="hover:text-ink-3">
          ← Back to sign in
        </Link>
      </p>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/auth/components/RegistrationDisabled.tsx
git commit -m "feat(web/auth): add RegistrationDisabled message"
```

---

### Task 15: RegisterForm component

**Files:**
- Create: `web/src/features/auth/components/RegisterForm.tsx`
- Test: `web/src/features/auth/components/RegisterForm.test.tsx`

Поведение:
- Поля: `email` (валидный), `displayName` (≥1), `password` (≥10 с inline-подсказкой).
- На 409 `email_taken` → top-of-form «An account with this email already exists.».
- На 403 `registration_disabled` → колбэк `onRegistrationDisabled()` (route переключит UI).
- На 400 `weak_password` → inline-ошибка под `password`.
- На success → `onSuccess()`.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/features/auth/components/RegisterForm.test.tsx
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { clearRefreshToken } from "@/features/auth/storage";
import { RegisterForm } from "./RegisterForm";

function setup() {
  const onSuccess = vi.fn();
  const onRegistrationDisabled = vi.fn();
  render(
    <RegisterForm onSuccess={onSuccess} onRegistrationDisabled={onRegistrationDisabled} />,
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
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
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
              email: "a@b.c",
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
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
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
          { code: "email_taken", message: "taken" },
          { status: 409 },
        ),
      ),
    );

    const { user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "p".repeat(10));
    await user.click(screen.getByRole("button", { name: /create account/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/email already exists/i);
  });

  it("on 403: calls onRegistrationDisabled", async () => {
    server.use(
      http.post("/api/auth/register", () =>
        HttpResponse.json(
          { code: "registration_disabled", message: "off" },
          { status: 403 },
        ),
      ),
    );

    const { onRegistrationDisabled, user } = setup();
    await user.type(screen.getByLabelText(/email/i), "a@b.c");
    await user.type(screen.getByLabelText(/display name/i), "Alice");
    await user.type(screen.getByLabelText(/password/i), "p".repeat(10));
    await user.click(screen.getByRole("button", { name: /create account/i }));

    await vi.waitFor(() => expect(onRegistrationDisabled).toHaveBeenCalledTimes(1));
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test -- RegisterForm`
Expected: FAIL with import error.

- [ ] **Step 3: Implement**

```tsx
// web/src/features/auth/components/RegisterForm.tsx
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { register as registerApi } from "@/features/auth/api";
import { ApiError } from "@/shared/api/errors";

const schema = z.object({
  email: z.string().email("Please enter a valid email address"),
  displayName: z.string().min(1, "Display name is required"),
  password: z.string().min(10, "Password must be at least 10 characters"),
});

type FormValues = z.infer<typeof schema>;

type Props = {
  onSuccess: () => void;
  onRegistrationDisabled: () => void;
};

export function RegisterForm({ onSuccess, onRegistrationDisabled }: Props) {
  const [topError, setTopError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", displayName: "", password: "" },
  });

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    try {
      await registerApi(values);
      onSuccess();
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 403 && err.code === "registration_disabled") {
          onRegistrationDisabled();
          return;
        }
        if (err.status === 409) {
          setTopError("An account with this email already exists.");
          return;
        }
        if (err.status === 400 && err.code === "weak_password") {
          setError("password", { message: "Password is too weak" });
          return;
        }
        if (err.status >= 500) {
          setTopError("Service unavailable. Please try again.");
          return;
        }
        setTopError(err.message || "Something went wrong");
        return;
      }
      setTopError("Something went wrong");
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-5">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-4 py-3 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="email" className="label-sc text-ink-3">
          Email
        </Label>
        <Input
          id="email"
          type="email"
          autoComplete="email"
          aria-invalid={errors.email ? "true" : "false"}
          aria-describedby={errors.email ? "email-error" : undefined}
          {...register("email")}
        />
        {errors.email && (
          <p id="email-error" className="text-sm text-vermillion-dark">
            {errors.email.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="displayName" className="label-sc text-ink-3">
          Display name
        </Label>
        <Input
          id="displayName"
          autoComplete="nickname"
          aria-invalid={errors.displayName ? "true" : "false"}
          aria-describedby={errors.displayName ? "displayName-error" : undefined}
          {...register("displayName")}
        />
        {errors.displayName && (
          <p id="displayName-error" className="text-sm text-vermillion-dark">
            {errors.displayName.message}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="password" className="label-sc text-ink-3">
          Password
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete="new-password"
          aria-invalid={errors.password ? "true" : "false"}
          aria-describedby="password-help"
          {...register("password")}
        />
        <p id="password-help" className="text-sm text-muted">
          {errors.password ? (
            <span className="text-vermillion-dark">{errors.password.message}</span>
          ) : (
            "At least 10 characters."
          )}
        </p>
      </div>

      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Creating account…" : "Create account"}
      </Button>

      <p className="label-sc text-center text-muted">
        <Link to="/login" className="hover:text-ink-3">
          ← Back to sign in
        </Link>
      </p>
    </form>
  );
}
```

- [ ] **Step 4: Run tests**

Run: `npm run test -- RegisterForm`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/auth/components/RegisterForm.tsx web/src/features/auth/components/RegisterForm.test.tsx
git commit -m "feat(web/auth): add RegisterForm with weak-password and email-taken handling"
```

---

### Task 16: Wire RegisterForm into /register route

**Files:**
- Modify: `web/src/routes/register.tsx`

Route хранит local state: `disabled: boolean`. Если `disabled` — рендерит `<RegistrationDisabled/>`, иначе `<RegisterForm/>`. Колбэк `onRegistrationDisabled` от формы переключает state.

- [ ] **Step 1: Edit**

```tsx
// web/src/routes/register.tsx
import { useState } from "react";
import { useNavigate } from "react-router";
import { RegisterForm } from "@/features/auth/components/RegisterForm";
import { RegistrationDisabled } from "@/features/auth/components/RegistrationDisabled";

export default function RegisterRoute() {
  const [disabled, setDisabled] = useState(false);
  const navigate = useNavigate();

  if (disabled) return <RegistrationDisabled />;

  return (
    <RegisterForm
      onSuccess={() => {
        navigate("/library", { replace: true });
      }}
      onRegistrationDisabled={() => setDisabled(true)}
    />
  );
}
```

- [ ] **Step 2: Run tests + typecheck**

Run: `npm run test` and `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/routes/register.tsx
git commit -m "feat(web): wire RegisterForm + RegistrationDisabled switch into /register"
```

---

## Группа 6 — Logout: Toaster + UserMenu

### Task 17: Sonner Toaster wrapper

**Files:**
- Create: `web/src/shared/ui/sonner.tsx`
- Modify: `web/src/App.tsx`

Стандартный shadcn-обвес: тонкий компонент `<Toaster />` поверх `sonner`. Mount внутри `<BootstrapGate>`. Position top-right, paper styling делаем через `richColors=false` + Tailwind в `toastOptions.className`.

- [ ] **Step 1: Create wrapper**

```tsx
// web/src/shared/ui/sonner.tsx
import { Toaster as SonnerToaster } from "sonner";

export function Toaster() {
  return (
    <SonnerToaster
      position="top-right"
      richColors={false}
      toastOptions={{
        className:
          "border border-rule bg-paper-2 text-ink font-body text-sm shadow-md",
      }}
    />
  );
}
```

- [ ] **Step 2: Mount in App.tsx**

In `web/src/App.tsx`, add import:
```tsx
import { Toaster } from "@/shared/ui/sonner";
```

And update the App default export:
```tsx
export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BootstrapGate>
        <RouterProvider router={router} />
        <Toaster />
      </BootstrapGate>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 3: Typecheck + tests**

Run: `npm run typecheck` and `npm run test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/shared/ui/sonner.tsx web/src/App.tsx
git commit -m "feat(web): add sonner Toaster and mount in App"
```

---

### Task 18: shadcn dropdown-menu copy

**Files:**
- Create: `web/src/shared/ui/dropdown-menu.tsx`

Стандартный shadcn-snippet для Radix `DropdownMenu`, адаптированный к style file. Используем минимальный набор примитивов: `Root`, `Trigger`, `Portal`, `Content`, `Item`, `Label`, `Separator`. Без sub-menu, checkbox, radio — пока не нужны.

- [ ] **Step 1: Implement**

```tsx
// web/src/shared/ui/dropdown-menu.tsx
import * as React from "react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { cn } from "@/shared/lib/cn";

function DropdownMenu(props: React.ComponentProps<typeof DropdownMenuPrimitive.Root>) {
  return <DropdownMenuPrimitive.Root data-slot="dropdown-menu" {...props} />;
}

function DropdownMenuTrigger(
  props: React.ComponentProps<typeof DropdownMenuPrimitive.Trigger>,
) {
  return <DropdownMenuPrimitive.Trigger data-slot="dropdown-menu-trigger" {...props} />;
}

function DropdownMenuPortal(
  props: React.ComponentProps<typeof DropdownMenuPrimitive.Portal>,
) {
  return <DropdownMenuPrimitive.Portal data-slot="dropdown-menu-portal" {...props} />;
}

function DropdownMenuContent({
  className,
  sideOffset = 6,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Content>) {
  return (
    <DropdownMenuPortal>
      <DropdownMenuPrimitive.Content
        data-slot="dropdown-menu-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 min-w-[180px] overflow-hidden rounded-none border border-rule bg-paper-2 p-1 shadow-md",
          "data-[state=open]:animate-in data-[state=closed]:animate-out",
          className,
        )}
        {...props}
      />
    </DropdownMenuPortal>
  );
}

function DropdownMenuItem({
  className,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Item>) {
  return (
    <DropdownMenuPrimitive.Item
      data-slot="dropdown-menu-item"
      className={cn(
        "relative flex cursor-default select-none items-center gap-2 px-3 py-2 text-sm text-ink-2 outline-none",
        "data-[highlighted]:bg-paper data-[disabled]:opacity-50 data-[disabled]:pointer-events-none",
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuLabel({
  className,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Label>) {
  return (
    <DropdownMenuPrimitive.Label
      data-slot="dropdown-menu-label"
      className={cn("label-sc px-3 py-2 text-muted", className)}
      {...props}
    />
  );
}

function DropdownMenuSeparator({
  className,
  ...props
}: React.ComponentProps<typeof DropdownMenuPrimitive.Separator>) {
  return (
    <DropdownMenuPrimitive.Separator
      data-slot="dropdown-menu-separator"
      className={cn("my-1 h-px bg-rule", className)}
      {...props}
    />
  );
}

export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuPortal,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
};
```

- [ ] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/shared/ui/dropdown-menu.tsx
git commit -m "feat(web/ui): add shadcn dropdown-menu component"
```

---

### Task 19: useLogout hook

**Files:**
- Create: `web/src/features/auth/use-logout.ts`

Хук возвращает `logout()`-функцию, которая:
1. Зовёт `POST /auth/logout` (best-effort — на сетевую ошибку молча игнорируем).
2. `clearRefreshToken()`.
3. `useAuthStore.clearSession()`.
4. `queryClient.clear()`.
5. `navigate('/login', { replace: true })`.
6. `toast.success('Signed out')`.

- [ ] **Step 1: Implement**

```ts
// web/src/features/auth/use-logout.ts
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { logout as logoutApi } from "./api";
import { clearRefreshToken } from "./storage";
import { useAuthStore } from "./store";

export function useLogout(): () => Promise<void> {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  return async () => {
    try {
      await logoutApi();
    } catch {
      // best-effort — locally we still clear
    }
    clearRefreshToken();
    useAuthStore.getState().clearSession();
    queryClient.clear();
    toast.success("Signed out");
    navigate("/login", { replace: true });
  };
}
```

- [ ] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/auth/use-logout.ts
git commit -m "feat(web/auth): add useLogout hook"
```

---

### Task 20: UserMenu component

**Files:**
- Create: `web/src/features/auth/components/UserMenu.tsx`

Кнопка-trigger — 36px квадрат с инициалом `user.displayName[0]` Cormorant. Dropdown показывает: label `displayName` + `email` (muted), separator, item «Sign out».

- [ ] **Step 1: Implement**

```tsx
// web/src/features/auth/components/UserMenu.tsx
import { useAuthStore } from "@/features/auth/store";
import { useLogout } from "@/features/auth/use-logout";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/shared/ui/dropdown-menu";

export function UserMenu() {
  const user = useAuthStore((s) => s.user);
  const logout = useLogout();

  if (!user) return null;

  const initial = user.displayName.charAt(0).toUpperCase() || "·";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Open user menu"
        className="h-9 w-9 rounded-none border border-rule bg-paper flex items-center justify-center hover:bg-paper-2 outline-none focus-visible:ring-2 focus-visible:ring-ink-3"
      >
        <span className="font-display text-lg text-ink">{initial}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>
          <span className="block text-ink-2 normal-case text-sm font-body tracking-normal">
            {user.displayName}
          </span>
          <span className="block text-xs text-muted normal-case tracking-normal mt-0.5">
            {user.email}
          </span>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void logout()}>Sign out</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

- [ ] **Step 2: Typecheck**

Run: `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/auth/components/UserMenu.tsx
git commit -m "feat(web/auth): add UserMenu with initial trigger and sign-out item"
```

---

### Task 21: Replace placeholder user square in Topbar with UserMenu

**Files:**
- Modify: `web/src/shared/layout/Topbar.tsx`

- [ ] **Step 1: Edit**

```tsx
// web/src/shared/layout/Topbar.tsx
import { Menu, Plus } from "lucide-react";
import { UserMenu } from "@/features/auth/components/UserMenu";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
  return (
    <header className="sticky top-0 z-10 h-16 bg-paper-2 border-b border-rule flex items-center px-4 lg:px-6">
      <button
        type="button"
        onClick={onMenuClick}
        aria-label="Open navigation"
        className="icon-btn lg:hidden"
      >
        <Menu className="h-5 w-5" strokeWidth={1.5} />
      </button>

      <div className="ml-auto flex items-center gap-3">
        <button
          type="button"
          aria-label="Add link"
          className="icon-btn"
          onClick={() => {
            // wired up in Library plan
            console.warn("Add Link not implemented yet");
          }}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <UserMenu />
      </div>
    </header>
  );
}
```

- [ ] **Step 2: Run tests + typecheck**

Run: `npm run test` and `npm run typecheck`
Expected: PASS.

- [ ] **Step 3: Smoke-test in dev (requires backend)**

Run backend in another terminal (`go run ./cmd/linktheca-server` or via docker compose).
Run `npm run dev` in `web/`.
Open `http://localhost:5173/` → FullPageSpinner briefly → `/login`.
Register or log in with seeded credentials.
Verify:
1. After login, redirects to `/library`.
2. Topbar right has user initial.
3. Click initial → dropdown shows displayName + email + «Sign out».
4. Click «Sign out» → toast «Signed out», back at `/login`, refresh page — stays at `/login` (refresh token cleared).

If you don't have a backend handy, this manual smoke test can be skipped — the unit/component tests cover the contract. Note this in the commit message.

- [ ] **Step 4: Commit**

```bash
git add web/src/shared/layout/Topbar.tsx
git commit -m "feat(web): mount UserMenu in Topbar"
```

---

## Группа 7 — Финальная проверка

### Task 22: Final sanity-проверка + lint + build

**Files:** —

- [ ] **Step 1: Full test run**

Run from `web/`:
```bash
npm run test
```
Expected: ALL pass, including:
- `apiFetch` (existing 6 + 5 new refresh tests = 11)
- `useAuthStore` (3)
- `storage` (4)
- `auth api` (5)
- `useBootstrap` (3)
- `ProtectedRoute` (3)
- `Sidebar` (1, existing)
- `errors` (existing)
- `LoginForm` (4)
- `RegisterForm` (4)

- [ ] **Step 2: Lint**

```bash
npm run lint
```
Expected: 0 warnings.

- [ ] **Step 3: Typecheck**

```bash
npm run typecheck
```
Expected: 0 errors.

- [ ] **Step 4: Build**

```bash
npm run build
```
Expected: succeeds, produces `dist/` with `index.html` and assets bundle.

- [ ] **Step 5: Spec coverage check**

Walk through spec sections 3 and 4. Each requirement should be implementable from this plan:

| Spec section | Implemented by task |
|---|---|
| 3 — Refresh-token in localStorage `linktheca.refresh` | Task 3 (storage), Task 4 (api consumes), Task 5 (refresh uses) |
| 3 — Access token in Zustand only | Task 4, Task 5 (no localStorage writes for access) |
| 3 — `apiFetch` refresh-on-401 singleton + retry once | Task 5 |
| 3 — Refresh-failure → clearSession + redirect via ProtectedRoute | Task 5 + Task 8 |
| 3 — Bootstrap on App mount, status='bootstrapping' until done | Task 7 + Task 9 |
| 3 — `<ProtectedRoute>`: FullPageSpinner / redirect / Outlet | Task 8 |
| 3 — Zod-парсинг `/auth/me`, `/auth/login`, `/auth/refresh` always | Task 2 + Task 4 + Task 5 |
| 3 — TanStack Query config (existed in foundation) | Foundation Task 14 |
| 3 — Logout: POST `/auth/logout`, clearSession, queryClient.clear, redirect | Task 17 (toaster) + Task 19 (use-logout) + Task 20–21 (wire) |
| 4 — Route tree with `/login`, `/register`, `/library*` protected | Task 9 |
| 4 — `from`-location preserved on login redirect | Task 13 |
| 4 — LoginForm: email valid, password ≥1, 401 inline, 5xx «Service unavailable» | Task 12 |
| 4 — Link «Create account →» under login | Task 12 |
| 4 — RegisterForm: email/display_name/password ≥10 | Task 15 |
| 4 — 403 → full-page «Registration disabled» | Tasks 14 + 15 + 16 |
| 4 — Public layout: paper-surface, masthead Cormorant italic, .rule-double | Tasks 10 + 11 |
| 4 — User-menu Cormorant initial → dropdown → logout | Task 20 + Task 21 |
| 4 — Toaster (sonner) used by logout | Task 17 + Task 19 |

If any line above doesn't match the implementation, fix it before declaring complete.

- [ ] **Step 6: Final commit (optional housekeeping)**

If anything turned up — fix and commit. Otherwise this task is just a checkpoint.

---

## Сводный self-review

**Coverage:**
- All sections 3 and 4 of the spec covered by tasks 1–21 (see Task 22 table).
- Tasks NOT included: TanStack Query config (already in foundation), `parseInDev` Zod helper (Library concern — moved to library plan), search input (out of scope per spec), Settings UI (out of scope), all Library screens (out of scope).

**Placeholder scan:**
- No «TBD»/«TODO»/«implement later» in steps.
- All code blocks complete and runnable.
- Error-handling specified literally (status codes, messages).

**Type consistency:**
- `User` (camelCase: `displayName`, `isAdmin`) defined in store, used by `mapUser`, `useAuthStore.setSession`, `LoginForm`, `RegisterForm`, `UserMenu`.
- `AuthResponseSchema.tokens.access_token`/`refresh_token` (snake_case from backend) consistently parsed and mapped throughout (api.ts, performRefresh).
- `apiFetch` signature `<T>(path, init?, opts?)` matches calls in `api.ts` (3 args used: `apiFetch<T>(path, init)`) and recursive retry call (`apiFetch<T>(path, init, { _retry: true })`).
- `LoginForm.onSuccess: () => void` matches `LoginRoute` usage.
- `RegisterForm.onSuccess + onRegistrationDisabled` both `() => void` match `RegisterRoute` usage.
- `useLogout()` returns `() => Promise<void>`; consumed by `UserMenu` as `() => void logout()` (fire-and-forget).

**Risks:**
- Browser `localStorage` disabled (private mode, security policy) → storage helpers swallow errors; user can still log in but won't persist across reloads. Acceptable per spec.
- Concurrent refresh from two tabs is NOT handled here (BroadcastChannel sync explicitly out of scope per spec section 3). Each tab refreshes independently — backend's refresh-rotation handles the resulting churn.

---

## Следующие шаги

После approval и merge этого плана — переход к `2026-05-XX-frontend-library.md`:
- Library list (filters, infinite query, card grid).
- Add Link modal (long-running POST с pending-progress UI).
- Reader view (meta + content parallel queries, drop-cap, scroll-to-read).
- Optimistic updates для favorite/mark-read.
- Edit (note) + Delete (confirm + invalidate).
- Wire «+ Add Link» Topbar button.
