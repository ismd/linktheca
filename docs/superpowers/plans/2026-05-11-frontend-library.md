# Frontend Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Поверх готового foundation+auth собрать Library UI: список с фильтрами (state pills + favorite toggle), пагинация через `useInfiniteQuery`, Add-link modal с трёхстадийным «прогрессом», reader-view (метаданные + prose + drop-cap + reading-progress bar), mark-as-read по скроллу 90%, favorite-toggle (optimistic), delete с AlertDialog. После плана пользователь может сохранить ссылку, посмотреть её в reader'е, отметить как избранное / прочитанное и удалить.

**Architecture:**
- API-слой в `features/library/api.ts` — тонкие функции поверх `apiFetch`. Zod-парсинг в dev (через `parseInDev`), сырая структура от backend'а (snake_case) сразу маппится в camelCase в TS-типах.
- TanStack Query на всё чтение: `useLibraryQuery` (infinite) для списка, `useLibraryItemDetailQuery` для reader'a. Мутации — `useSaveLink`, `useUpdateItem`, `useDeleteItem`. Favorite-toggle оптимистичный (snapshot → rollback). Mark-as-read и delete — без optimistic, just invalidate.
- Фильтры (`state`, `favorite`) живут в URL через `useSearchParams`, что даёт bookmarkable views.
- Add-link открывается из двух мест (Topbar `+` и EmptyState CTA) → состояние «открыто» в маленьком Zustand-сторе `useAddLinkStore`. Dialog рендерится на уровне `AppLayout`.
- Reader-страница параллельно грузит meta-item и `:id/content` (две query'и). Скролл-listener в самой странице ставит `state='read'` один раз при достижении 90%.
- Sort-опция из спека («oldest first») **отложена** — backend сортирует только `saved_at DESC`. Добавим UI-control когда появится бэкенд-параметр.

**Tech Stack:** TanStack Query v5 (infinite), React Hook Form + Zod, sonner toasts, Radix Dialog/AlertDialog, lucide-react, date-fns (новая dep). Уже есть: zod, zustand, react-router v7, MSW для тестов.

**Spec:** `docs/superpowers/specs/2026-05-08-frontend-foundation-library-design.md` секции 3–4 (Cross-cutting/optimistic/toasts + Library list + Add Link modal + Reader view).

---

## Контракт backend'a (фиксируем здесь, чтобы не путаться)

| Method | Path | Auth | Запрос | Ответ |
|---|---|---|---|---|
| `POST` | `/library` | Bearer | `{ "url": string }` | `201` + `Item` |
| `GET` | `/library` | Bearer | query: `state=unread|read|archived`, `favorite=true|false`, `limit` (1–100, default 50), `offset` (default 0) | `200` + `{ items: Item[], total: number }` |
| `GET` | `/library/{id}` | Bearer | — | `200` + `Item` |
| `GET` | `/library/{id}/content` | Bearer | — | `200` + `ItemDetail` (= `Item` + `content: ArticleContent`) |
| `PATCH` | `/library/{id}` | Bearer | `{ state?, is_favorite?, note? }` | `200` + `Item` |
| `DELETE` | `/library/{id}` | Bearer | — | `204` |

Ошибки:
- `400 bad_request`
- `404 not_found`
- `409 already_saved` (только на POST `/library` при попытке сохранить уже сохранённое)
- `500 internal`

`Item` (snake_case JSON):
```json
{
  "id": 1, "user_id": 1, "content_id": 1,
  "state": "unread|read|archived",
  "is_favorite": false,
  "note": null,
  "saved_at": "2026-05-11T...Z",
  "read_at": null,
  "url": "https://...",
  "title": "…",
  "excerpt": "…",
  "reading_time_seconds": 480
}
```

`ArticleContent` добавляет: `canonical_url`, `byline`, `text`, `html`, `lang`, `fetched_at`, `fetch_error`.

В TS используем camelCase + Date-объекты после маппинга — см. Task 3.

---

## Файловая структура (создаётся/меняется этим планом)

```
web/
  package.json                                       # +date-fns
  src/
    App.tsx                                          # (no change)
    routes/
      library._index.tsx                             # REWRITE — wire list + filters + add-link button
      library.$id.tsx                                # REWRITE — reader view
    features/
      library/
        api.ts                                       # NEW
        api.test.ts                                  # NEW
        schemas.ts                                   # NEW — RawItem/RawItemDetail + mappers
        types.ts                                     # NEW — camelCase Item/ItemDetail
        time.ts                                      # NEW — relativeFromNow, readingTimeLabel
        time.test.ts                                 # NEW
        image.ts                                     # NEW — gradientClassFor(id) → 'img-1'..'img-10'
        use-library.ts                               # NEW — list/get/detail hooks
        use-library.test.tsx                         # NEW
        use-mutations.ts                             # NEW — save/update/delete mutations
        use-mutations.test.tsx                       # NEW
        use-add-link-store.ts                        # NEW — Zustand store for dialog open state
        components/
          LibraryCard.tsx                            # NEW
          LibraryCard.test.tsx                       # NEW
          SkeletonCard.tsx                           # NEW
          EmptyState.tsx                             # NEW
          ErrorPanel.tsx                             # NEW
          FilterBar.tsx                              # NEW
          FilterBar.test.tsx                         # NEW
          LibraryGrid.tsx                            # NEW
          LibraryGrid.test.tsx                       # NEW
          AddLinkDialog.tsx                          # NEW
          AddLinkDialog.test.tsx                     # NEW
          ReadingProgress.tsx                        # NEW
          ReaderHeader.tsx                           # NEW
          ReaderActions.tsx                          # NEW
          ReaderActions.test.tsx                     # NEW
          DeleteConfirm.tsx                          # NEW
          useMarkReadOnScroll.ts                     # NEW
          useMarkReadOnScroll.test.tsx               # NEW
    shared/
      layout/
        Topbar.tsx                                   # MODIFY — Add Link button calls store.open()
      api/
        client.ts                                    # (no change)
```

---

## Группа 1 — Зависимости и API-слой

### Task 1: Установить date-fns

**Files:**
- Modify: `web/package.json`
- Modify: `web/package-lock.json`

- [x] **Step 1: Install**

Run from `/home/ismd/coding/linktheca/web`:
```bash
npm install date-fns
```

Expected: latest stable, на момент написания `^3.6` или `^4.x`. Caret-пин как у других зависимостей.

- [x] **Step 2: Verify**

```bash
npm run typecheck
npm run test
```
Expected: PASS, изменений в коде ещё нет.

- [x] **Step 3: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "deps(web): add date-fns for relative time"
```

---

### Task 2: Типы Library (camelCase frontend-side)

**Files:**
- Create: `web/src/features/library/types.ts`

- [x] **Step 1: Write the file**

```ts
// web/src/features/library/types.ts

export type LibraryState = "unread" | "read" | "archived";

export type LibraryItem = {
  id: number;
  state: LibraryState;
  isFavorite: boolean;
  note: string | null;
  savedAt: Date;
  readAt: Date | null;
  url: string;
  title: string | null;
  excerpt: string | null;
  readingTimeSeconds: number | null;
};

export type ArticleContent = {
  id: number;
  url: string;
  canonicalUrl: string | null;
  title: string | null;
  byline: string | null;
  excerpt: string | null;
  text: string | null;
  html: string | null;
  lang: string | null;
  readingTimeSeconds: number | null;
  fetchedAt: Date;
  fetchError: string | null;
};

export type LibraryItemDetail = LibraryItem & {
  content: ArticleContent;
};

export type ListPage = {
  items: LibraryItem[];
  total: number;
};

export type FilterParams = {
  state?: LibraryState;
  favorite?: boolean;
};

export const PAGE_SIZE = 20;
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS — никто ещё не импортирует, изменений нет.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/types.ts
git commit -m "feat(web/library): add camelCase types for items/detail/page"
```

---

### Task 3: Zod schemas + mapper

**Files:**
- Create: `web/src/features/library/schemas.ts`

Backend returns snake_case. Парсим raw shape Zod'ом, маппим в `LibraryItem`/`LibraryItemDetail` функциями `mapItem`/`mapDetail`.

- [x] **Step 1: Write the file**

```ts
// web/src/features/library/schemas.ts
import { z } from "zod";
import type {
  LibraryItem,
  LibraryItemDetail,
  ArticleContent,
  ListPage,
} from "./types";

export const RawItemSchema = z.object({
  id: z.number().int(),
  state: z.enum(["unread", "read", "archived"]),
  is_favorite: z.boolean(),
  note: z.string().nullable().optional(),
  saved_at: z.string(),
  read_at: z.string().nullable().optional(),
  url: z.string(),
  title: z.string().nullable().optional(),
  excerpt: z.string().nullable().optional(),
  reading_time_seconds: z.number().int().nullable().optional(),
});

export const RawContentSchema = z.object({
  id: z.number().int(),
  url: z.string(),
  canonical_url: z.string().nullable().optional(),
  title: z.string().nullable().optional(),
  byline: z.string().nullable().optional(),
  excerpt: z.string().nullable().optional(),
  text: z.string().nullable().optional(),
  html: z.string().nullable().optional(),
  lang: z.string().nullable().optional(),
  reading_time_seconds: z.number().int().nullable().optional(),
  fetched_at: z.string(),
  fetch_error: z.string().nullable().optional(),
});

export const RawItemDetailSchema = RawItemSchema.extend({
  content: RawContentSchema,
});

export const RawListPageSchema = z.object({
  items: z.array(RawItemSchema),
  total: z.number().int(),
});

export type RawItem = z.infer<typeof RawItemSchema>;
export type RawItemDetail = z.infer<typeof RawItemDetailSchema>;
export type RawListPage = z.infer<typeof RawListPageSchema>;

function nn<T>(v: T | null | undefined): T | null {
  return v ?? null;
}

export function mapItem(raw: RawItem): LibraryItem {
  return {
    id: raw.id,
    state: raw.state,
    isFavorite: raw.is_favorite,
    note: nn(raw.note),
    savedAt: new Date(raw.saved_at),
    readAt: raw.read_at ? new Date(raw.read_at) : null,
    url: raw.url,
    title: nn(raw.title),
    excerpt: nn(raw.excerpt),
    readingTimeSeconds: nn(raw.reading_time_seconds),
  };
}

export function mapContent(raw: z.infer<typeof RawContentSchema>): ArticleContent {
  return {
    id: raw.id,
    url: raw.url,
    canonicalUrl: nn(raw.canonical_url),
    title: nn(raw.title),
    byline: nn(raw.byline),
    excerpt: nn(raw.excerpt),
    text: nn(raw.text),
    html: nn(raw.html),
    lang: nn(raw.lang),
    readingTimeSeconds: nn(raw.reading_time_seconds),
    fetchedAt: new Date(raw.fetched_at),
    fetchError: nn(raw.fetch_error),
  };
}

export function mapDetail(raw: RawItemDetail): LibraryItemDetail {
  return { ...mapItem(raw), content: mapContent(raw.content) };
}

export function mapListPage(raw: RawListPage): ListPage {
  return { items: raw.items.map(mapItem), total: raw.total };
}
```

- [x] **Step 2: Verify typecheck**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/schemas.ts
git commit -m "feat(web/library): add Zod schemas and snake→camel mappers"
```

---

### Task 4: API functions (failing tests first)

**Files:**
- Create: `web/src/features/library/api.ts`
- Create: `web/src/features/library/api.test.ts`

- [x] **Step 1: Write the failing tests**

```ts
// web/src/features/library/api.test.ts
import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  listLibrary,
  getLibraryItem,
  getLibraryDetail,
  saveLink,
  updateItem,
  deleteItem,
} from "./api";

const rawItem = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/a",
  title: "Example",
  excerpt: "Some excerpt",
  reading_time_seconds: 480,
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("library api", () => {
  it("listLibrary sends limit/offset/state/favorite as query params", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [rawItem()], total: 1 });
      }),
    );

    const page = await listLibrary({
      limit: 20,
      offset: 40,
      state: "unread",
      favorite: true,
    });

    expect(capturedUrl).toContain("limit=20");
    expect(capturedUrl).toContain("offset=40");
    expect(capturedUrl).toContain("state=unread");
    expect(capturedUrl).toContain("favorite=true");
    expect(page.total).toBe(1);
    expect(page.items[0].savedAt).toBeInstanceOf(Date);
    expect(page.items[0].isFavorite).toBe(false);
  });

  it("listLibrary omits filter params when not set", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );

    await listLibrary({ limit: 20, offset: 0 });
    expect(capturedUrl).not.toContain("state=");
    expect(capturedUrl).not.toContain("favorite=");
  });

  it("getLibraryItem maps the response", async () => {
    server.use(
      http.get("/api/library/42", () => HttpResponse.json(rawItem({ id: 42 }))),
    );
    const item = await getLibraryItem(42);
    expect(item.id).toBe(42);
    expect(item.savedAt).toBeInstanceOf(Date);
  });

  it("getLibraryDetail maps item+content", async () => {
    server.use(
      http.get("/api/library/7/content", () =>
        HttpResponse.json({
          ...rawItem({ id: 7 }),
          content: {
            id: 99,
            url: "https://example.com/a",
            canonical_url: null,
            title: "Example",
            byline: "By Someone",
            excerpt: "Some excerpt",
            text: "Full text",
            html: "<p>Full text</p>",
            lang: "en",
            reading_time_seconds: 480,
            fetched_at: "2026-05-10T12:00:00Z",
            fetch_error: null,
          },
        }),
      ),
    );
    const detail = await getLibraryDetail(7);
    expect(detail.content.html).toBe("<p>Full text</p>");
    expect(detail.content.fetchedAt).toBeInstanceOf(Date);
  });

  it("saveLink POSTs { url } and returns mapped item", async () => {
    let captured: { url: string } | null = null;
    server.use(
      http.post("/api/library", async ({ request }) => {
        captured = (await request.json()) as { url: string };
        return HttpResponse.json(rawItem({ id: 5 }), { status: 201 });
      }),
    );

    const item = await saveLink("https://example.com/a");
    expect(captured).toEqual({ url: "https://example.com/a" });
    expect(item.id).toBe(5);
  });

  it("updateItem PATCHes and maps response", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ id: 3, is_favorite: true }));
      }),
    );

    const item = await updateItem(3, { isFavorite: true });
    expect(captured).toEqual({ is_favorite: true });
    expect(item.isFavorite).toBe(true);
  });

  it("updateItem maps state and note correctly", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ id: 3, state: "read", note: "hi" }));
      }),
    );

    await updateItem(3, { state: "read", note: "hi" });
    expect(captured).toEqual({ state: "read", note: "hi" });
  });

  it("deleteItem DELETEs", async () => {
    let called = false;
    server.use(
      http.delete("/api/library/9", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await deleteItem(9);
    expect(called).toBe(true);
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/api.test.ts
```
Expected: FAIL (file `./api` does not exist).

- [x] **Step 3: Implement api.ts**

```ts
// web/src/features/library/api.ts
import { apiFetch } from "@/shared/api/client";
import {
  RawItemSchema,
  RawItemDetailSchema,
  RawListPageSchema,
  mapItem,
  mapDetail,
  mapListPage,
} from "./schemas";
import type {
  LibraryItem,
  LibraryItemDetail,
  ListPage,
  LibraryState,
} from "./types";

export type ListArgs = {
  limit: number;
  offset: number;
  state?: LibraryState;
  favorite?: boolean;
};

function buildQuery(args: ListArgs): string {
  const p = new URLSearchParams();
  p.set("limit", String(args.limit));
  p.set("offset", String(args.offset));
  if (args.state) p.set("state", args.state);
  if (args.favorite !== undefined) p.set("favorite", String(args.favorite));
  return p.toString();
}

function parseInDev<T>(schema: { parse: (x: unknown) => T }, data: unknown): T {
  if (import.meta.env.DEV || import.meta.env.MODE === "test") {
    return schema.parse(data);
  }
  return data as T;
}

export async function listLibrary(args: ListArgs): Promise<ListPage> {
  const raw = await apiFetch<unknown>(`/library?${buildQuery(args)}`);
  return mapListPage(parseInDev(RawListPageSchema, raw));
}

export async function getLibraryItem(id: number): Promise<LibraryItem> {
  const raw = await apiFetch<unknown>(`/library/${id}`);
  return mapItem(parseInDev(RawItemSchema, raw));
}

export async function getLibraryDetail(id: number): Promise<LibraryItemDetail> {
  const raw = await apiFetch<unknown>(`/library/${id}/content`);
  return mapDetail(parseInDev(RawItemDetailSchema, raw));
}

export async function saveLink(url: string): Promise<LibraryItem> {
  const raw = await apiFetch<unknown>(`/library`, {
    method: "POST",
    body: JSON.stringify({ url }),
  });
  return mapItem(parseInDev(RawItemSchema, raw));
}

export type UpdateInput = {
  state?: LibraryState;
  isFavorite?: boolean;
  note?: string | null;
};

export async function updateItem(
  id: number,
  input: UpdateInput,
): Promise<LibraryItem> {
  const body: Record<string, unknown> = {};
  if (input.state !== undefined) body.state = input.state;
  if (input.isFavorite !== undefined) body.is_favorite = input.isFavorite;
  if (input.note !== undefined) body.note = input.note;
  const raw = await apiFetch<unknown>(`/library/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return mapItem(parseInDev(RawItemSchema, raw));
}

export async function deleteItem(id: number): Promise<void> {
  await apiFetch<void>(`/library/${id}`, { method: "DELETE" });
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/api.test.ts
```
Expected: PASS (8 tests).

- [x] **Step 5: Run typecheck**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 6: Commit**

```bash
git add web/src/features/library/api.ts web/src/features/library/api.test.ts
git commit -m "feat(web/library): add API client with Zod validation"
```

---

### Task 5: time helpers (relative date, reading-time label)

**Files:**
- Create: `web/src/features/library/time.ts`
- Create: `web/src/features/library/time.test.ts`

- [x] **Step 1: Write the failing tests**

```ts
// web/src/features/library/time.test.ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { relativeFromNow, readingTimeLabel } from "./time";

const fixedNow = new Date("2026-05-11T12:00:00Z");

afterEach(() => {
  vi.useRealTimers();
});

describe("relativeFromNow", () => {
  it("returns 'today' for today", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-11T08:00:00Z");
    expect(relativeFromNow(d)).toBe("today");
  });

  it("returns 'yesterday' for ~24h ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-10T12:00:00Z");
    expect(relativeFromNow(d)).toBe("yesterday");
  });

  it("returns '3 days ago' for 3 days ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-05-08T12:00:00Z");
    expect(relativeFromNow(d)).toBe("3 days ago");
  });

  it("returns 'Apr 11' for >7 days ago in same year", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2026-04-11T12:00:00Z");
    expect(relativeFromNow(d)).toBe("Apr 11");
  });

  it("returns 'Apr 11, 2025' for previous year", () => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
    const d = new Date("2025-04-11T12:00:00Z");
    expect(relativeFromNow(d)).toBe("Apr 11, 2025");
  });
});

describe("readingTimeLabel", () => {
  it("returns '1 min read' for <90s", () => {
    expect(readingTimeLabel(45)).toBe("1 min read");
    expect(readingTimeLabel(89)).toBe("1 min read");
  });

  it("rounds to nearest minute", () => {
    expect(readingTimeLabel(180)).toBe("3 min read");
    expect(readingTimeLabel(330)).toBe("6 min read");
  });

  it("returns '— read' for null", () => {
    expect(readingTimeLabel(null)).toBe("— read");
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/time.test.ts
```
Expected: FAIL.

- [x] **Step 3: Implement**

```ts
// web/src/features/library/time.ts
import {
  differenceInCalendarDays,
  format,
  isSameYear,
} from "date-fns";

export function relativeFromNow(d: Date, now: Date = new Date()): string {
  const days = differenceInCalendarDays(now, d);
  if (days <= 0) return "today";
  if (days === 1) return "yesterday";
  if (days < 7) return `${days} days ago`;
  return isSameYear(d, now) ? format(d, "MMM d") : format(d, "MMM d, yyyy");
}

export function readingTimeLabel(seconds: number | null): string {
  if (seconds == null) return "— read";
  const minutes = Math.max(1, Math.round(seconds / 60));
  return `${minutes} min read`;
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/time.test.ts
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/time.ts web/src/features/library/time.test.ts
git commit -m "feat(web/library): add relative-time and reading-time helpers"
```

---

### Task 6: image gradient picker

**Files:**
- Create: `web/src/features/library/image.ts`

- [x] **Step 1: Write the file**

```ts
// web/src/features/library/image.ts
// Deterministic mapping id → one of the 10 mock-image gradient classes from globals.css.
// Used while we have no real og:image extraction.

export function gradientClassFor(id: number): string {
  const bucket = ((id - 1) % 10 + 10) % 10; // safe for negative/zero
  return `img-${bucket + 1}`;
}
```

- [x] **Step 2: Verify typecheck**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/image.ts
git commit -m "feat(web/library): add deterministic gradient class picker"
```

---

## Группа 2 — TanStack Query hooks

### Task 7: List query (useInfiniteQuery)

**Files:**
- Create: `web/src/features/library/use-library.ts`
- Create: `web/src/features/library/use-library.test.tsx`

- [x] **Step 1: Write failing tests**

```tsx
// web/src/features/library/use-library.test.tsx
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { useLibraryQuery, useLibraryItemDetailQuery } from "./use-library";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const rawItem = (id: number) => ({
  id,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: `2026-05-${String(id).padStart(2, "0")}T12:00:00Z`,
  read_at: null,
  url: `https://example.com/${id}`,
  title: `Title ${id}`,
  excerpt: "ex",
  reading_time_seconds: 120,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("useLibraryQuery", () => {
  it("loads first page with given filters", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({
          items: [rawItem(1), rawItem(2)],
          total: 2,
        });
      }),
    );

    const { result } = renderHook(
      () => useLibraryQuery({ state: "unread" }),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(capturedUrl).toContain("state=unread");
    expect(capturedUrl).toContain("offset=0");
    expect(result.current.items).toHaveLength(2);
    expect(result.current.total).toBe(2);
    expect(result.current.hasMore).toBe(false);
  });

  it("computes hasMore=true when total > items loaded", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawItem(i + 1)),
          total: 55,
        }),
      ),
    );

    const { result } = renderHook(() => useLibraryQuery({}), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasMore).toBe(true);
    expect(result.current.items).toHaveLength(20);
  });
});

describe("useLibraryItemDetailQuery", () => {
  it("loads item detail by id", async () => {
    server.use(
      http.get("/api/library/3/content", () =>
        HttpResponse.json({
          ...rawItem(3),
          content: {
            id: 99,
            url: "https://example.com/3",
            canonical_url: null,
            title: "T",
            byline: null,
            excerpt: null,
            text: "body",
            html: "<p>body</p>",
            lang: "en",
            reading_time_seconds: 120,
            fetched_at: "2026-05-10T12:00:00Z",
            fetch_error: null,
          },
        }),
      ),
    );

    const { result } = renderHook(() => useLibraryItemDetailQuery(3), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.content.html).toBe("<p>body</p>");
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/use-library.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```ts
// web/src/features/library/use-library.ts
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { listLibrary, getLibraryDetail, getLibraryItem } from "./api";
import { PAGE_SIZE, type FilterParams, type ListPage } from "./types";

export const libraryKeys = {
  all: ["library"] as const,
  list: (filters: FilterParams) => ["library", "list", filters] as const,
  item: (id: number) => ["library", "item", id] as const,
  detail: (id: number) => ["library", "detail", id] as const,
};

export function useLibraryQuery(filters: FilterParams) {
  const query = useInfiniteQuery({
    queryKey: libraryKeys.list(filters),
    queryFn: ({ pageParam }) =>
      listLibrary({
        limit: PAGE_SIZE,
        offset: pageParam as number,
        state: filters.state,
        favorite: filters.favorite,
      }),
    initialPageParam: 0,
    getNextPageParam: (last: ListPage, all: ListPage[]) => {
      const loaded = all.reduce((s, p) => s + p.items.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });

  const items = (query.data?.pages ?? []).flatMap((p) => p.items);
  const total = query.data?.pages?.[0]?.total ?? 0;
  const hasMore = query.hasNextPage ?? false;

  return {
    items,
    total,
    hasMore,
    isLoading: query.isLoading,
    isSuccess: query.isSuccess,
    isError: query.isError,
    error: query.error,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}

export function useLibraryItemQuery(id: number) {
  return useQuery({
    queryKey: libraryKeys.item(id),
    queryFn: () => getLibraryItem(id),
  });
}

export function useLibraryItemDetailQuery(id: number) {
  return useQuery({
    queryKey: libraryKeys.detail(id),
    queryFn: () => getLibraryDetail(id),
  });
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/use-library.test.tsx
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/use-library.ts web/src/features/library/use-library.test.tsx
git commit -m "feat(web/library): add list/item/detail query hooks"
```

---

### Task 8: Mutations (save / update with optimistic favorite / delete)

**Files:**
- Create: `web/src/features/library/use-mutations.ts`
- Create: `web/src/features/library/use-mutations.test.tsx`

- [x] **Step 1: Write failing tests**

```tsx
// web/src/features/library/use-mutations.test.tsx
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { libraryKeys } from "./use-library";
import {
  useSaveLink,
  useUpdateItem,
  useDeleteItem,
} from "./use-mutations";
import type { ListPage } from "./types";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper };
}

const rawItem = (id: number, overrides: Record<string, unknown> = {}) => ({
  id,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: `https://example.com/${id}`,
  title: `Title ${id}`,
  excerpt: "ex",
  reading_time_seconds: 120,
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("useSaveLink", () => {
  it("on success invalidates list query", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json(rawItem(7), { status: 201 }),
      ),
    );

    const { qc, wrapper } = makeWrapper();
    // seed list cache
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      { pages: [{ items: [], total: 0 }], pageParams: [0] },
    );

    const { result } = renderHook(() => useSaveLink(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync("https://example.com/7");
    });

    await waitFor(() =>
      expect(qc.getQueryState(libraryKeys.list({}))?.isInvalidated).toBe(true),
    );
  });
});

describe("useUpdateItem (optimistic favorite)", () => {
  it("optimistically updates list cache on isFavorite toggle", async () => {
    let resolve!: (v: Response) => void;
    const slow = new Promise<Response>((r) => (resolve = r));
    server.use(http.patch("/api/library/1", () => slow));

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      {
        pages: [
          {
            items: [
              {
                id: 1,
                state: "unread",
                isFavorite: false,
                note: null,
                savedAt: new Date(),
                readAt: null,
                url: "u",
                title: "t",
                excerpt: "e",
                readingTimeSeconds: 60,
              },
            ],
            total: 1,
          },
        ],
        pageParams: [0],
      },
    );

    const { result } = renderHook(() => useUpdateItem(), { wrapper });

    act(() => {
      result.current.mutate({ id: 1, input: { isFavorite: true } });
    });

    await waitFor(() => {
      const data = qc.getQueryData<{ pages: ListPage[] }>(libraryKeys.list({}));
      expect(data?.pages[0].items[0].isFavorite).toBe(true);
    });

    // resolve to make mutation finish
    resolve(
      HttpResponse.json(rawItem(1, { is_favorite: true })) as unknown as Response,
    );
  });

  it("rolls back optimistic update on failure", async () => {
    server.use(
      http.patch("/api/library/1", () =>
        HttpResponse.json({ code: "internal", message: "boom" }, { status: 500 }),
      ),
    );

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData<{ pages: ListPage[]; pageParams: number[] }>(
      libraryKeys.list({}),
      {
        pages: [
          {
            items: [
              {
                id: 1,
                state: "unread",
                isFavorite: false,
                note: null,
                savedAt: new Date(),
                readAt: null,
                url: "u",
                title: "t",
                excerpt: "e",
                readingTimeSeconds: 60,
              },
            ],
            total: 1,
          },
        ],
        pageParams: [0],
      },
    );

    const { result } = renderHook(() => useUpdateItem(), { wrapper });

    await act(async () => {
      try {
        await result.current.mutateAsync({ id: 1, input: { isFavorite: true } });
      } catch {
        // expected
      }
    });

    const data = qc.getQueryData<{ pages: ListPage[] }>(libraryKeys.list({}));
    expect(data?.pages[0].items[0].isFavorite).toBe(false);
  });
});

describe("useDeleteItem", () => {
  it("invalidates list and removes detail cache", async () => {
    server.use(
      http.delete("/api/library/5", () => new HttpResponse(null, { status: 204 })),
    );

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(libraryKeys.detail(5), { id: 5 });
    qc.setQueryData(libraryKeys.list({}), {
      pages: [{ items: [], total: 0 }],
      pageParams: [0],
    });

    const { result } = renderHook(() => useDeleteItem(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(5);
    });

    expect(qc.getQueryData(libraryKeys.detail(5))).toBeUndefined();
    expect(qc.getQueryState(libraryKeys.list({}))?.isInvalidated).toBe(true);
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/use-mutations.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```ts
// web/src/features/library/use-mutations.ts
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { saveLink, updateItem, deleteItem, type UpdateInput } from "./api";
import { libraryKeys } from "./use-library";
import type { LibraryItem, ListPage } from "./types";

type InfiniteListData = {
  pages: ListPage[];
  pageParams: unknown[];
};

export function useSaveLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => saveLink(url),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: libraryKeys.all });
    },
  });
}

type UpdateArgs = { id: number; input: UpdateInput };

type RollbackCtx = {
  previousLists: [readonly unknown[], InfiniteListData | undefined][];
  previousItem: LibraryItem | undefined;
};

export function useUpdateItem() {
  const qc = useQueryClient();
  return useMutation<LibraryItem, Error, UpdateArgs, RollbackCtx>({
    mutationFn: ({ id, input }) => updateItem(id, input),
    onMutate: async ({ id, input }) => {
      await qc.cancelQueries({ queryKey: libraryKeys.all });

      const previousLists = qc.getQueriesData<InfiniteListData>({
        queryKey: ["library", "list"],
      });
      const previousItem = qc.getQueryData<LibraryItem>(libraryKeys.item(id));

      // Patch every cached list page that contains this item.
      previousLists.forEach(([key, data]) => {
        if (!data) return;
        qc.setQueryData<InfiniteListData>(key, {
          ...data,
          pages: data.pages.map((page) => ({
            ...page,
            items: page.items.map((it) =>
              it.id === id ? applyPatch(it, input) : it,
            ),
          })),
        });
      });

      // Patch single-item cache if present.
      if (previousItem) {
        qc.setQueryData<LibraryItem>(libraryKeys.item(id), applyPatch(previousItem, input));
      }

      return { previousLists, previousItem };
    },
    onError: (_err, vars, ctx) => {
      ctx?.previousLists.forEach(([key, data]) => {
        qc.setQueryData(key, data);
      });
      if (ctx?.previousItem !== undefined) {
        qc.setQueryData(libraryKeys.item(vars.id), ctx.previousItem);
      }
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: libraryKeys.detail(vars.id) });
    },
  });
}

function applyPatch(item: LibraryItem, input: UpdateInput): LibraryItem {
  return {
    ...item,
    state: input.state ?? item.state,
    isFavorite: input.isFavorite ?? item.isFavorite,
    note: input.note === undefined ? item.note : input.note,
  };
}

export function useDeleteItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteItem(id),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: libraryKeys.detail(id) });
      qc.removeQueries({ queryKey: libraryKeys.item(id) });
      qc.invalidateQueries({ queryKey: ["library", "list"] });
    },
  });
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/use-mutations.test.tsx
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/use-mutations.ts web/src/features/library/use-mutations.test.tsx
git commit -m "feat(web/library): add save/update/delete mutations with optimistic favorite"
```

---

### Task 9: Add-link dialog state store

**Files:**
- Create: `web/src/features/library/use-add-link-store.ts`

- [x] **Step 1: Write the file**

```ts
// web/src/features/library/use-add-link-store.ts
import { create } from "zustand";

type State = {
  isOpen: boolean;
  open: () => void;
  close: () => void;
};

export const useAddLinkStore = create<State>((set) => ({
  isOpen: false,
  open: () => set({ isOpen: true }),
  close: () => set({ isOpen: false }),
}));
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/use-add-link-store.ts
git commit -m "feat(web/library): add Zustand store for AddLink dialog state"
```

---

## Группа 3 — Список (карточки, фильтры, grid)

### Task 10: SkeletonCard

**Files:**
- Create: `web/src/features/library/components/SkeletonCard.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/SkeletonCard.tsx
export function SkeletonCard() {
  return (
    <article className="flex flex-col h-full" data-testid="library-skeleton-card">
      <div className="skeleton aspect-[16/10] w-full mb-5" />
      <div className="skeleton h-4 w-1/3 mb-3" />
      <div className="skeleton h-7 w-5/6 mb-4" />
      <div className="skeleton h-4 w-full mb-1" />
      <div className="skeleton h-4 w-2/3" />
    </article>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/SkeletonCard.tsx
git commit -m "feat(web/library): add SkeletonCard for list loading state"
```

---

### Task 11: ErrorPanel

**Files:**
- Create: `web/src/features/library/components/ErrorPanel.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/ErrorPanel.tsx
import { Button } from "@/shared/ui/button";

type Props = {
  message: string;
  onRetry: () => void;
};

export function ErrorPanel({ message, onRetry }: Props) {
  return (
    <div
      role="alert"
      className="border border-vermillion bg-paper-2 px-6 py-8 text-center"
    >
      <p className="font-body text-ink-3 mb-4">{message}</p>
      <Button variant="outline" onClick={onRetry}>
        Try again
      </Button>
    </div>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/ErrorPanel.tsx
git commit -m "feat(web/library): add ErrorPanel for retryable failures"
```

---

### Task 12: EmptyState

**Files:**
- Create: `web/src/features/library/components/EmptyState.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/EmptyState.tsx
import { Button } from "@/shared/ui/button";
import { useAddLinkStore } from "../use-add-link-store";

type Props = {
  filtered: boolean;
};

export function EmptyState({ filtered }: Props) {
  const open = useAddLinkStore((s) => s.open);

  if (filtered) {
    return (
      <div className="text-center py-20 border border-dashed border-rule">
        <p className="label-sc text-muted-foreground mb-3">No matches</p>
        <p className="font-body italic text-ink-3">
          No items match the current filters.
        </p>
      </div>
    );
  }

  return (
    <div className="text-center py-24">
      <h2 className="display-tight text-4xl text-ink mb-3">Nothing here yet</h2>
      <p className="label-sc text-muted-foreground mb-8">Save your first link →</p>
      <Button onClick={open}>+ Add link</Button>
    </div>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/EmptyState.tsx
git commit -m "feat(web/library): add EmptyState component with Add-link CTA"
```

---

### Task 13: LibraryCard

**Files:**
- Create: `web/src/features/library/components/LibraryCard.tsx`
- Create: `web/src/features/library/components/LibraryCard.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/library/components/LibraryCard.test.tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { LibraryCard } from "./LibraryCard";
import type { LibraryItem } from "../types";

const baseItem: LibraryItem = {
  id: 7,
  state: "unread",
  isFavorite: false,
  note: null,
  savedAt: new Date("2026-05-10T12:00:00Z"),
  readAt: null,
  url: "https://example.com/article",
  title: "Example Title",
  excerpt: "Some short excerpt",
  readingTimeSeconds: 180,
};

describe("LibraryCard", () => {
  it("renders title, excerpt, reading-time and host link", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    expect(screen.getByText("Example Title")).toBeInTheDocument();
    expect(screen.getByText(/Some short excerpt/)).toBeInTheDocument();
    expect(screen.getByText(/3 min read/)).toBeInTheDocument();
    expect(screen.getByText(/example\.com/)).toBeInTheDocument();
  });

  it("wraps the entire card in a Link to /library/:id", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={baseItem} />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/library/7");
  });

  it("shows the favorite mark when isFavorite=true", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, isFavorite: true }} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText(/favorite/i)).toBeInTheDocument();
  });

  it("shows the read stamp when state='read'", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, state: "read" }} />
      </MemoryRouter>,
    );
    expect(screen.getByText(/read/i)).toBeInTheDocument();
  });

  it("falls back to URL when title is null", () => {
    render(
      <MemoryRouter>
        <LibraryCard item={{ ...baseItem, title: null }} />
      </MemoryRouter>,
    );
    expect(screen.getByText("https://example.com/article")).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/LibraryCard.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```tsx
// web/src/features/library/components/LibraryCard.tsx
import { Link } from "react-router";
import { Star } from "lucide-react";
import { gradientClassFor } from "../image";
import { relativeFromNow, readingTimeLabel } from "../time";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
};

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

export function LibraryCard({ item }: Props) {
  const title = item.title ?? item.url;
  const isRead = item.state === "read";
  return (
    <Link
      to={`/library/${item.id}`}
      className="feed-card group block"
      aria-label={title}
    >
      <article className="flex flex-col h-full">
        <div
          className={`${gradientClassFor(item.id)} relative overflow-hidden mb-5`}
          style={{ aspectRatio: "16 / 10" }}
        >
          <div className="absolute top-3 left-3 flex gap-2">
            <span
              className={`stamp bg-paper/95 stamp-flat ${
                isRead ? "text-sage" : "text-vermillion"
              }`}
            >
              {isRead ? "✓ read" : "✦ saved"}
            </span>
            {item.isFavorite && (
              <span
                aria-label="favorite"
                className="stamp bg-paper/95 stamp-flat text-ochre"
              >
                <Star
                  className="inline h-3 w-3 mr-1"
                  strokeWidth={2}
                  aria-hidden="true"
                />
                favorite
              </span>
            )}
          </div>
          <div className="absolute bottom-3 right-3 label-sc text-paper/80">
            {readingTimeLabel(item.readingTimeSeconds)}
          </div>
        </div>

        <div className="flex items-center gap-2 mb-3 flex-wrap">
          <span className="label-sc text-muted-foreground">{host(item.url)}</span>
          <span className="label-sc text-muted-foreground">·</span>
          <span className="label-sc text-muted-foreground">
            {relativeFromNow(item.savedAt)}
          </span>
        </div>

        <h2 className="card-title display-tight text-2xl text-ink leading-[1.1] mb-3">
          {title}
        </h2>

        {item.excerpt && (
          <p className="font-body text-ink-3 leading-[1.55] line-clamp-3">
            {item.excerpt}
          </p>
        )}
      </article>
    </Link>
  );
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/LibraryCard.test.tsx
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/LibraryCard.tsx web/src/features/library/components/LibraryCard.test.tsx
git commit -m "feat(web/library): add LibraryCard with gradient hero, stamps, host/time row"
```

---

### Task 14: FilterBar

**Files:**
- Create: `web/src/features/library/components/FilterBar.tsx`
- Create: `web/src/features/library/components/FilterBar.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/library/components/FilterBar.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FilterBar } from "./FilterBar";

describe("FilterBar", () => {
  it("highlights the active state pill", () => {
    render(
      <FilterBar
        state="unread"
        favorite={false}
        onChange={() => {}}
      />,
    );
    const unread = screen.getByRole("button", { name: /^unread$/i });
    expect(unread).toHaveAttribute("aria-pressed", "true");
    const all = screen.getByRole("button", { name: /^all$/i });
    expect(all).toHaveAttribute("aria-pressed", "false");
  });

  it("clicking a state pill calls onChange with that state (or undefined for All)", async () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        state="unread"
        favorite={false}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /^read$/i }));
    expect(onChange).toHaveBeenLastCalledWith({ state: "read", favorite: false });

    await userEvent.click(screen.getByRole("button", { name: /^all$/i }));
    expect(onChange).toHaveBeenLastCalledWith({ state: undefined, favorite: false });
  });

  it("toggling favorites flips the favorite flag", async () => {
    const onChange = vi.fn();
    render(
      <FilterBar
        state={undefined}
        favorite={false}
        onChange={onChange}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /favorites only/i }),
    );
    expect(onChange).toHaveBeenLastCalledWith({ state: undefined, favorite: true });
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/FilterBar.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```tsx
// web/src/features/library/components/FilterBar.tsx
import { Star } from "lucide-react";
import type { FilterParams, LibraryState } from "../types";

type Props = {
  state: LibraryState | undefined;
  favorite: boolean;
  onChange: (next: FilterParams) => void;
};

const STATES: { label: string; value: LibraryState | undefined }[] = [
  { label: "All", value: undefined },
  { label: "Unread", value: "unread" },
  { label: "Read", value: "read" },
  { label: "Archived", value: "archived" },
];

export function FilterBar({ state, favorite, onChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3 py-4 border-b border-rule">
      <div className="flex flex-wrap gap-1" role="group" aria-label="State filter">
        {STATES.map((opt) => {
          const active = state === opt.value;
          return (
            <button
              key={opt.label}
              type="button"
              aria-pressed={active}
              onClick={() =>
                onChange({ state: opt.value, favorite: favorite || undefined })
              }
              className={
                active
                  ? "px-3 py-1.5 label-sc bg-ink text-paper"
                  : "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2"
              }
            >
              {opt.label}
            </button>
          );
        })}
      </div>

      <div className="ml-auto">
        <button
          type="button"
          onClick={() => onChange({ state, favorite: !favorite })}
          className={
            favorite
              ? "px-3 py-1.5 label-sc bg-ochre text-paper inline-flex items-center gap-2"
              : "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2 inline-flex items-center gap-2"
          }
        >
          <Star className="h-3.5 w-3.5" strokeWidth={1.5} aria-hidden="true" />
          Favorites only
        </button>
      </div>
    </div>
  );
}
```

> Note: `favorite: favorite || undefined` keeps the URL clean — `?favorite=false` becomes no param at all when the toggle is off.

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/FilterBar.test.tsx
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/FilterBar.tsx web/src/features/library/components/FilterBar.test.tsx
git commit -m "feat(web/library): add FilterBar with state pills and favorite toggle"
```

---

### Task 15: LibraryGrid (loading/empty/error/data + Load more)

**Files:**
- Create: `web/src/features/library/components/LibraryGrid.tsx`
- Create: `web/src/features/library/components/LibraryGrid.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/library/components/LibraryGrid.test.tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { LibraryGrid } from "./LibraryGrid";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </MemoryRouter>
  );
}

const rawItem = (id: number) => ({
  id,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: `https://example.com/${id}`,
  title: `Title ${id}`,
  excerpt: "ex",
  reading_time_seconds: 120,
});

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("LibraryGrid", () => {
  it("renders skeletons while loading", () => {
    server.use(http.get("/api/library", async () => {
      await new Promise(() => {}); // never resolve
      return HttpResponse.json({ items: [], total: 0 });
    }));
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(screen.getAllByTestId("library-skeleton-card").length).toBeGreaterThan(0);
  });

  it("renders empty state (CTA) when not filtered and no items", async () => {
    server.use(
      http.get("/api/library", () => HttpResponse.json({ items: [], total: 0 })),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(await screen.findByText(/nothing here yet/i)).toBeInTheDocument();
  });

  it("renders 'no matches' when filtered and empty", async () => {
    server.use(
      http.get("/api/library", () => HttpResponse.json({ items: [], total: 0 })),
    );
    render(<LibraryGrid filters={{ state: "read" }} />, { wrapper: wrapper() });
    expect(await screen.findByText(/no matches/i)).toBeInTheDocument();
  });

  it("renders cards and Load more when hasMore", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawItem(i + 1)),
          total: 30,
        }),
      ),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    await screen.findByText("Title 1");
    expect(screen.getByRole("button", { name: /load more/i })).toBeInTheDocument();
  });

  it("clicking Load more fetches the next page", async () => {
    let call = 0;
    server.use(
      http.get("/api/library", ({ request }) => {
        call += 1;
        const url = new URL(request.url);
        const offset = Number(url.searchParams.get("offset") ?? 0);
        return HttpResponse.json({
          items: [rawItem(offset + 1)],
          total: 2,
        });
      }),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    await screen.findByText("Title 1");
    await userEvent.click(screen.getByRole("button", { name: /load more/i }));
    await screen.findByText("Title 2");
    expect(call).toBeGreaterThanOrEqual(2);
  });

  it("shows ErrorPanel with retry on failure", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({ code: "internal", message: "boom" }, { status: 500 }),
      ),
    );
    render(<LibraryGrid filters={{}} />, { wrapper: wrapper() });
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /try again/i })).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/LibraryGrid.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```tsx
// web/src/features/library/components/LibraryGrid.tsx
import { useLibraryQuery } from "../use-library";
import type { FilterParams } from "../types";
import { LibraryCard } from "./LibraryCard";
import { SkeletonCard } from "./SkeletonCard";
import { EmptyState } from "./EmptyState";
import { ErrorPanel } from "./ErrorPanel";
import { Button } from "@/shared/ui/button";

type Props = {
  filters: FilterParams;
};

export function LibraryGrid({ filters }: Props) {
  const q = useLibraryQuery(filters);
  const filtered = filters.state !== undefined || filters.favorite === true;

  if (q.isLoading) {
    return (
      <div
        role="status"
        aria-label="Loading library"
        className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8"
      >
        {Array.from({ length: 6 }).map((_, i) => (
          <SkeletonCard key={i} />
        ))}
      </div>
    );
  }

  if (q.isError) {
    return (
      <ErrorPanel
        message={
          q.error instanceof Error ? q.error.message : "Failed to load library"
        }
        onRetry={() => q.refetch()}
      />
    );
  }

  if (q.items.length === 0) {
    return <EmptyState filtered={filtered} />;
  }

  return (
    <div className="flex flex-col gap-10">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
        {q.items.map((item) => (
          <LibraryCard key={item.id} item={item} />
        ))}
      </div>

      {q.hasMore && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={() => q.fetchNextPage()}
            disabled={q.isFetchingNextPage}
          >
            {q.isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/LibraryGrid.test.tsx
```
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/LibraryGrid.tsx web/src/features/library/components/LibraryGrid.test.tsx
git commit -m "feat(web/library): add LibraryGrid with loading/empty/error/load-more"
```

---

### Task 16: List route — wire filters to URL search params

**Files:**
- Modify: `web/src/routes/library._index.tsx`

- [x] **Step 1: Rewrite the file**

Replace the placeholder contents with:

```tsx
// web/src/routes/library._index.tsx
import { useSearchParams } from "react-router";
import { PageHeader } from "@/shared/layout/PageHeader";
import { FilterBar } from "@/features/library/components/FilterBar";
import { LibraryGrid } from "@/features/library/components/LibraryGrid";
import type { FilterParams, LibraryState } from "@/features/library/types";

const ALLOWED_STATES: LibraryState[] = ["unread", "read", "archived"];

function parseFilters(params: URLSearchParams): FilterParams {
  const state = params.get("state");
  const fav = params.get("favorite");
  return {
    state:
      state && (ALLOWED_STATES as string[]).includes(state)
        ? (state as LibraryState)
        : undefined,
    favorite: fav === "true" ? true : undefined,
  };
}

function nextSearch(current: URLSearchParams, next: FilterParams): URLSearchParams {
  const out = new URLSearchParams(current);
  if (next.state) out.set("state", next.state);
  else out.delete("state");
  if (next.favorite) out.set("favorite", "true");
  else out.delete("favorite");
  return out;
}

export default function LibraryListRoute() {
  const [params, setParams] = useSearchParams();
  const filters = parseFilters(params);

  return (
    <div>
      <PageHeader title="Library" subtitle="Your saved articles" />
      <div className="px-4 lg:px-8 pb-10">
        <FilterBar
          state={filters.state}
          favorite={filters.favorite === true}
          onChange={(next) => setParams(nextSearch(params, next), { replace: true })}
        />
        <div className="pt-8">
          <LibraryGrid filters={filters} />
        </div>
      </div>
    </div>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
npm run test
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/routes/library._index.tsx
git commit -m "feat(web/library): wire library list route with URL-driven filters"
```

---

## Группа 4 — Add-link modal

### Task 17: AddLinkDialog component

**Files:**
- Create: `web/src/features/library/components/AddLinkDialog.tsx`
- Create: `web/src/features/library/components/AddLinkDialog.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/library/components/AddLinkDialog.test.tsx
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { useAddLinkStore } from "../use-add-link-store";
import { AddLinkDialog } from "./AddLinkDialog";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
  useAddLinkStore.getState().close();
  vi.useRealTimers();
});

const rawItem = {
  id: 1,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/a",
  title: "T",
  excerpt: null,
  reading_time_seconds: null,
};

describe("AddLinkDialog", () => {
  it("does not render content when closed", () => {
    render(<AddLinkDialog />, { wrapper: wrapper() });
    expect(screen.queryByLabelText(/url/i)).not.toBeInTheDocument();
  });

  it("renders form when open", () => {
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    expect(screen.getByLabelText(/url/i)).toBeInTheDocument();
  });

  it("validates URL inline", async () => {
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "not a url");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByText(/valid url/i)).toBeInTheDocument();
  });

  it("on success: closes dialog and shows toast", async () => {
    server.use(
      http.post("/api/library", () => HttpResponse.json(rawItem, { status: 201 })),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() =>
      expect(useAddLinkStore.getState().isOpen).toBe(false),
    );
    expect(await screen.findByText(/saved to library/i)).toBeInTheDocument();
  });

  it("on 409 already_saved: shows specific error and stays open", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json(
          { code: "already_saved", message: "already" },
          { status: 409 },
        ),
      ),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/already in library/i);
    expect(useAddLinkStore.getState().isOpen).toBe(true);
  });

  it("on 5xx: shows generic error", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json({ code: "internal", message: "x" }, { status: 500 }),
      ),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/couldn't save/i);
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/AddLinkDialog.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```tsx
// web/src/features/library/components/AddLinkDialog.tsx
import { useState, useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useAddLinkStore } from "../use-add-link-store";
import { useSaveLink } from "../use-mutations";

const schema = z.object({
  url: z.string().url("Please enter a valid URL (including https://)"),
});
type FormValues = z.infer<typeof schema>;

const PROGRESS_STAGES = [
  "Fetching page…",
  "Extracting content…",
  "Saving to library…",
];

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "already_saved" || err.status === 409) {
      return "This article is already in your library";
    }
    if (err.status === 422) {
      return "Couldn't extract content from this URL";
    }
    if (err.status >= 500) {
      return "Couldn't save — please try again";
    }
    return err.message || "Couldn't save — please try again";
  }
  return "Couldn't save — please try again";
}

export function AddLinkDialog() {
  const isOpen = useAddLinkStore((s) => s.isOpen);
  const close = useAddLinkStore((s) => s.close);
  const save = useSaveLink();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { url: "" },
  });

  const [topError, setTopError] = useState<string | null>(null);
  const [stage, setStage] = useState(0);

  useEffect(() => {
    if (!save.isPending) return;
    setStage(0);
    const t1 = setTimeout(() => setStage(1), 1500);
    const t2 = setTimeout(() => setStage(2), 3500);
    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
    };
  }, [save.isPending]);

  useEffect(() => {
    if (!isOpen) {
      reset({ url: "" });
      setTopError(null);
    }
  }, [isOpen, reset]);

  const onSubmit = handleSubmit(async ({ url }) => {
    setTopError(null);
    try {
      await save.mutateAsync(url);
      toast.success("Saved to library");
      close();
    } catch (err) {
      setTopError(mapError(err));
    }
  });

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent className="paper-surface">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Add a link</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Paste a URL — we'll fetch and save the article.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
          {topError && (
            <div
              role="alert"
              className="border border-vermillion bg-paper-2 px-3 py-2 text-sm text-vermillion-dark"
            >
              {topError}
            </div>
          )}

          <div className="flex flex-col gap-2">
            <Label htmlFor="add-link-url" className="label-sc text-ink-3">
              URL
            </Label>
            <Input
              id="add-link-url"
              autoFocus
              placeholder="https://…"
              aria-invalid={errors.url ? "true" : "false"}
              aria-describedby={errors.url ? "add-link-url-error" : undefined}
              disabled={save.isPending}
              {...register("url")}
            />
            {errors.url && (
              <p
                id="add-link-url-error"
                className="text-sm text-vermillion-dark"
              >
                {errors.url.message}
              </p>
            )}
          </div>

          {save.isPending && (
            <div className="rounded border border-rule bg-paper-2/50 px-4 py-3">
              <p className="label-sc text-ink-3">{PROGRESS_STAGES[stage]}</p>
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={close}
              disabled={save.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/AddLinkDialog.test.tsx
```
Expected: PASS (6 tests).

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/AddLinkDialog.tsx web/src/features/library/components/AddLinkDialog.test.tsx
git commit -m "feat(web/library): add AddLinkDialog with three-stage progress and error mapping"
```

---

### Task 18: Mount AddLinkDialog in AppLayout

**Files:**
- Modify: `web/src/routes/__app.tsx`

- [x] **Step 1: Modify the file**

```tsx
// web/src/routes/__app.tsx
import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";
import { AddLinkDialog } from "@/features/library/components/AddLinkDialog";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
      <AddLinkDialog />
    </AppShell>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
npm run test
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/routes/__app.tsx
git commit -m "feat(web): mount AddLinkDialog in AppLayout"
```

---

### Task 19: Wire Topbar Add-link button

**Files:**
- Modify: `web/src/shared/layout/Topbar.tsx`

- [x] **Step 1: Replace the placeholder onClick**

```tsx
// web/src/shared/layout/Topbar.tsx
import { Menu, Plus } from "lucide-react";
import { UserMenu } from "@/features/auth/components/UserMenu";
import { useAddLinkStore } from "@/features/library/use-add-link-store";

type Props = {
  onMenuClick: () => void;
};

export function Topbar({ onMenuClick }: Props) {
  const openAddLink = useAddLinkStore((s) => s.open);

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
          onClick={openAddLink}
        >
          <Plus className="h-5 w-5" strokeWidth={1.5} />
        </button>

        <UserMenu />
      </div>
    </header>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
npm run test
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/shared/layout/Topbar.tsx
git commit -m "feat(web): wire Topbar add-link button to AddLink dialog"
```

---

## Группа 5 — Reader

### Task 20: ReadingProgress bar

**Files:**
- Create: `web/src/features/library/components/ReadingProgress.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/ReadingProgress.tsx
import { useEffect, useState } from "react";

export function ReadingProgress() {
  const [progress, setProgress] = useState(0);

  useEffect(() => {
    function update() {
      const doc = document.documentElement;
      const scrolled = doc.scrollTop;
      const max = doc.scrollHeight - doc.clientHeight;
      setProgress(max > 0 ? Math.min(1, scrolled / max) : 0);
    }
    update();
    window.addEventListener("scroll", update, { passive: true });
    window.addEventListener("resize", update);
    return () => {
      window.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
    };
  }, []);

  return (
    <div
      aria-hidden="true"
      className="fixed top-0 left-0 right-0 h-[2px] bg-transparent z-20 pointer-events-none"
    >
      <div
        className="h-full bg-vermillion origin-left transition-[width] duration-75"
        style={{ width: `${(progress * 100).toFixed(2)}%` }}
      />
    </div>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/ReadingProgress.tsx
git commit -m "feat(web/library): add ReadingProgress bar bound to window scroll"
```

---

### Task 21: ReaderHeader

**Files:**
- Create: `web/src/features/library/components/ReaderHeader.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/ReaderHeader.tsx
import { relativeFromNow, readingTimeLabel } from "../time";
import type { LibraryItemDetail } from "../types";

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

type Props = {
  detail: LibraryItemDetail;
};

export function ReaderHeader({ detail }: Props) {
  const title = detail.content.title ?? detail.title ?? detail.url;
  return (
    <header className="mb-10">
      <h1 className="display-tight text-4xl md:text-5xl text-ink leading-[1.05] mb-6">
        {title}
      </h1>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-2 font-body italic text-base text-muted-foreground">
        {detail.content.byline && (
          <span>
            by{" "}
            <span className="not-italic text-ink">{detail.content.byline}</span>
          </span>
        )}
        <span>{host(detail.url)}</span>
        <span>{relativeFromNow(detail.savedAt)}</span>
        <span>{readingTimeLabel(detail.content.readingTimeSeconds ?? detail.readingTimeSeconds)}</span>
      </div>
    </header>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/ReaderHeader.tsx
git commit -m "feat(web/library): add ReaderHeader with title and meta row"
```

---

### Task 22: DeleteConfirm

**Files:**
- Create: `web/src/features/library/components/DeleteConfirm.tsx`

- [x] **Step 1: Write the file**

```tsx
// web/src/features/library/components/DeleteConfirm.tsx
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/ui/alert-dialog";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  pending?: boolean;
};

export function DeleteConfirm({ open, onOpenChange, onConfirm, pending }: Props) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete this article?</AlertDialogTitle>
          <AlertDialogDescription>
            It will be removed from your library. The original page stays online.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            disabled={pending}
            className="bg-vermillion text-paper hover:bg-vermillion-dark"
          >
            {pending ? "Deleting…" : "Delete"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
```

- [x] **Step 2: Verify**

```bash
npm run typecheck
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/features/library/components/DeleteConfirm.tsx
git commit -m "feat(web/library): add DeleteConfirm alert dialog"
```

---

### Task 23: ReaderActions footer

**Files:**
- Create: `web/src/features/library/components/ReaderActions.tsx`
- Create: `web/src/features/library/components/ReaderActions.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
// web/src/features/library/components/ReaderActions.test.tsx
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { ReaderActions } from "./ReaderActions";
import type { LibraryItem } from "../types";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        {children}
        <Toaster />
      </QueryClientProvider>
    </MemoryRouter>
  );
}

const itemBase: LibraryItem = {
  id: 5,
  state: "unread",
  isFavorite: false,
  note: null,
  savedAt: new Date(),
  readAt: null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  readingTimeSeconds: 60,
};

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

const rawItem = (overrides: Record<string, unknown> = {}) => ({
  id: 5,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/x",
  title: "T",
  excerpt: null,
  reading_time_seconds: 60,
  ...overrides,
});

describe("ReaderActions", () => {
  it("toggle favorite calls PATCH with is_favorite=true", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ is_favorite: true }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /favorite/i }));
    await waitFor(() =>
      expect(captured).toEqual({ is_favorite: true }),
    );
  });

  it("mark-read button calls PATCH with state=read when unread", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "read" }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /mark as read/i }));
    await waitFor(() => expect(captured).toEqual({ state: "read" }));
  });

  it("mark-unread button shows when state=read and PATCHes back to unread", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "unread" }));
      }),
    );
    render(<ReaderActions item={{ ...itemBase, state: "read" }} />, {
      wrapper: wrapper(),
    });
    await userEvent.click(screen.getByRole("button", { name: /mark as unread/i }));
    await waitFor(() => expect(captured).toEqual({ state: "unread" }));
  });

  it("archive button calls PATCH with state=archived", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/5", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ state: "archived" }));
      }),
    );
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    await userEvent.click(screen.getByRole("button", { name: /archive/i }));
    await waitFor(() => expect(captured).toEqual({ state: "archived" }));
  });

  it("delete opens AlertDialog and on confirm calls DELETE then navigates", async () => {
    let called = false;
    server.use(
      http.delete("/api/library/5", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const onDeleted = vi.fn();
    render(<ReaderActions item={itemBase} onDeleted={onDeleted} />, {
      wrapper: wrapper(),
    });
    await userEvent.click(screen.getByRole("button", { name: /delete/i }));
    const confirm = await screen.findByRole("button", { name: "Delete" });
    await userEvent.click(confirm);
    await waitFor(() => expect(called).toBe(true));
    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it("open original opens external link in new tab", () => {
    render(<ReaderActions item={itemBase} />, { wrapper: wrapper() });
    const link = screen.getByRole("link", { name: /open original/i });
    expect(link).toHaveAttribute("href", "https://example.com/x");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/ReaderActions.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```tsx
// web/src/features/library/components/ReaderActions.tsx
import { useState } from "react";
import { toast } from "sonner";
import { ExternalLink, Star, Archive, Trash2, BookOpen, BookOpenCheck } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { useUpdateItem, useDeleteItem } from "../use-mutations";
import { DeleteConfirm } from "./DeleteConfirm";
import type { LibraryItem } from "../types";

type Props = {
  item: LibraryItem;
  onDeleted?: () => void;
};

export function ReaderActions({ item, onDeleted }: Props) {
  const update = useUpdateItem();
  const del = useDeleteItem();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const toggleFavorite = () =>
    update.mutate(
      { id: item.id, input: { isFavorite: !item.isFavorite } },
      {
        onError: () => toast.error("Couldn't update favorite"),
      },
    );

  const archive = () =>
    update.mutate(
      { id: item.id, input: { state: "archived" } },
      {
        onSuccess: () => toast.success("Archived"),
        onError: () => toast.error("Couldn't archive"),
      },
    );

  const toggleRead = () => {
    const next = item.state === "read" ? "unread" : "read";
    update.mutate(
      { id: item.id, input: { state: next } },
      {
        onError: () => toast.error("Couldn't update state"),
      },
    );
  };

  const confirmDelete = () => {
    del.mutate(item.id, {
      onSuccess: () => {
        toast.success("Deleted");
        setConfirmOpen(false);
        onDeleted?.();
      },
      onError: () => {
        toast.error("Couldn't delete");
        setConfirmOpen(false);
      },
    });
  };

  return (
    <div className="mt-16 pt-10 border-t-2 border-ink">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="outline" onClick={toggleRead}>
          {item.state === "read" ? (
            <BookOpen className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          ) : (
            <BookOpenCheck className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          )}
          {item.state === "read" ? "Mark as unread" : "Mark as read"}
        </Button>

        <Button variant="outline" onClick={toggleFavorite}>
          <Star
            className="h-4 w-4"
            strokeWidth={1.5}
            fill={item.isFavorite ? "currentColor" : "none"}
            aria-hidden="true"
          />
          {item.isFavorite ? "Favorited" : "Favorite"}
        </Button>

        <Button variant="outline" onClick={archive} disabled={item.state === "archived"}>
          <Archive className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          {item.state === "archived" ? "Archived" : "Archive"}
        </Button>

        <Button variant="ghost" asChild>
          <a href={item.url} target="_blank" rel="noopener noreferrer">
            <ExternalLink className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
            Open original
          </a>
        </Button>

        <Button
          variant="ghost"
          className="ml-auto text-vermillion-dark hover:text-vermillion"
          onClick={() => setConfirmOpen(true)}
        >
          <Trash2 className="h-4 w-4" strokeWidth={1.5} aria-hidden="true" />
          Delete
        </Button>
      </div>

      <DeleteConfirm
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        onConfirm={confirmDelete}
        pending={del.isPending}
      />
    </div>
  );
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/ReaderActions.test.tsx
```
Expected: PASS (4 tests).

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/ReaderActions.tsx web/src/features/library/components/ReaderActions.test.tsx
git commit -m "feat(web/library): add ReaderActions footer with favorite/archive/delete/open"
```

---

### Task 24: useMarkReadOnScroll hook

**Files:**
- Create: `web/src/features/library/components/useMarkReadOnScroll.ts`
- Create: `web/src/features/library/components/useMarkReadOnScroll.test.tsx`

- [x] **Step 1: Write failing tests**

```tsx
// web/src/features/library/components/useMarkReadOnScroll.test.tsx
import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMarkReadOnScroll } from "./useMarkReadOnScroll";

function setScrollMetrics(scrollTop: number, scrollHeight: number, clientHeight: number) {
  Object.defineProperty(document.documentElement, "scrollTop", {
    configurable: true,
    value: scrollTop,
  });
  Object.defineProperty(document.documentElement, "scrollHeight", {
    configurable: true,
    value: scrollHeight,
  });
  Object.defineProperty(document.documentElement, "clientHeight", {
    configurable: true,
    value: clientHeight,
  });
}

describe("useMarkReadOnScroll", () => {
  beforeEach(() => {
    setScrollMetrics(0, 1000, 500);
  });

  it("does not fire below threshold", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: true, onReach: fn }));
    setScrollMetrics(100, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).not.toHaveBeenCalled();
  });

  it("fires once at ≥90% of scrollable area", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: true, onReach: fn }));
    // scrollable area = scrollHeight - clientHeight = 500. 90% = 450.
    setScrollMetrics(460, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).toHaveBeenCalledTimes(1);

    setScrollMetrics(500, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).toHaveBeenCalledTimes(1); // still once
  });

  it("does nothing when disabled", () => {
    const fn = vi.fn();
    renderHook(() => useMarkReadOnScroll({ enabled: false, onReach: fn }));
    setScrollMetrics(500, 1000, 500);
    act(() => {
      window.dispatchEvent(new Event("scroll"));
    });
    expect(fn).not.toHaveBeenCalled();
  });
});
```

- [x] **Step 2: Run tests, see failure**

```bash
npm run test -- features/library/components/useMarkReadOnScroll.test.tsx
```
Expected: FAIL.

- [x] **Step 3: Implement**

```ts
// web/src/features/library/components/useMarkReadOnScroll.ts
import { useEffect, useRef } from "react";

const THRESHOLD = 0.9;

type Args = {
  enabled: boolean;
  onReach: () => void;
};

export function useMarkReadOnScroll({ enabled, onReach }: Args): void {
  const fired = useRef(false);

  useEffect(() => {
    if (!enabled) return;
    fired.current = false;

    function handler() {
      if (fired.current) return;
      const doc = document.documentElement;
      const max = doc.scrollHeight - doc.clientHeight;
      if (max <= 0) return;
      const ratio = doc.scrollTop / max;
      if (ratio >= THRESHOLD) {
        fired.current = true;
        onReach();
      }
    }

    window.addEventListener("scroll", handler, { passive: true });
    handler(); // run once in case already scrolled past
    return () => window.removeEventListener("scroll", handler);
  }, [enabled, onReach]);
}
```

- [x] **Step 4: Run tests, see PASS**

```bash
npm run test -- features/library/components/useMarkReadOnScroll.test.tsx
```
Expected: PASS (3 tests).

- [x] **Step 5: Commit**

```bash
git add web/src/features/library/components/useMarkReadOnScroll.ts web/src/features/library/components/useMarkReadOnScroll.test.tsx
git commit -m "feat(web/library): add useMarkReadOnScroll hook for auto mark-as-read"
```

---

### Task 25: Reader route — wire everything

**Files:**
- Modify: `web/src/routes/library.$id.tsx`

- [x] **Step 1: Rewrite the file**

```tsx
// web/src/routes/library.$id.tsx
import { useNavigate, useParams, Link } from "react-router";
import { useCallback } from "react";
import { useLibraryItemDetailQuery } from "@/features/library/use-library";
import { useUpdateItem } from "@/features/library/use-mutations";
import { ReadingProgress } from "@/features/library/components/ReadingProgress";
import { ReaderHeader } from "@/features/library/components/ReaderHeader";
import { ReaderActions } from "@/features/library/components/ReaderActions";
import { ErrorPanel } from "@/features/library/components/ErrorPanel";
import { useMarkReadOnScroll } from "@/features/library/components/useMarkReadOnScroll";
import { gradientClassFor } from "@/features/library/image";
import { ApiError } from "@/shared/api/errors";

function LoadingState() {
  return (
    <div className="max-w-[720px] mx-auto px-4 pt-10" aria-label="Loading article">
      <div className="skeleton h-10 w-3/4 mb-6" />
      <div className="skeleton h-4 w-1/2 mb-10" />
      <div className="skeleton h-[300px] w-full mb-10" />
      <div className="skeleton h-4 w-full mb-3" />
      <div className="skeleton h-4 w-5/6 mb-3" />
      <div className="skeleton h-4 w-4/6" />
    </div>
  );
}

export default function LibraryItemRoute() {
  const { id } = useParams();
  const itemId = Number(id);
  const navigate = useNavigate();
  const detail = useLibraryItemDetailQuery(itemId);
  const update = useUpdateItem();

  const onReach = useCallback(() => {
    if (!detail.data) return;
    if (detail.data.state !== "unread") return;
    update.mutate({ id: itemId, input: { state: "read" } });
  }, [detail.data, itemId, update]);

  useMarkReadOnScroll({
    enabled: detail.data?.state === "unread",
    onReach,
  });

  if (Number.isNaN(itemId)) {
    return (
      <div className="p-8">
        <p className="font-body text-ink-3">Invalid article id.</p>
      </div>
    );
  }

  if (detail.isLoading) return <LoadingState />;

  if (detail.isError) {
    const notFound =
      detail.error instanceof ApiError && detail.error.status === 404;
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10">
        {notFound ? (
          <div className="text-center py-20">
            <h1 className="display-tight text-4xl text-ink mb-3">
              Article not found
            </h1>
            <Link to="/library" className="label-sc text-vermillion">
              ← Back to library
            </Link>
          </div>
        ) : (
          <ErrorPanel
            message="Couldn't load this article"
            onRetry={() => detail.refetch()}
          />
        )}
      </div>
    );
  }

  const d = detail.data!;
  const fetchFailed = Boolean(d.content.fetchError);

  return (
    <>
      <ReadingProgress />
      <article className="max-w-[720px] mx-auto px-4 pt-8 pb-20">
        <Link
          to="/library"
          className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
        >
          ← Back to library
        </Link>

        <ReaderHeader detail={d} />

        <div
          className={`${gradientClassFor(d.id)} relative overflow-hidden w-full h-[280px] md:h-[360px] mb-10`}
        />

        {fetchFailed ? (
          <div className="border border-rule bg-paper-2 px-5 py-6">
            <p className="label-sc text-muted-foreground mb-2">Extraction failed</p>
            <p className="font-body text-ink-3 mb-4">
              We couldn't extract content from this page. Open the original to read it.
            </p>
            <p className="font-mono text-xs text-muted-foreground">
              {d.content.fetchError}
            </p>
          </div>
        ) : d.content.html ? (
          <div
            className="prose-reader drop-cap"
            dangerouslySetInnerHTML={{ __html: d.content.html }}
          />
        ) : d.content.text ? (
          <div className="prose-reader drop-cap whitespace-pre-wrap">
            {d.content.text}
          </div>
        ) : (
          <p className="font-body italic text-muted-foreground">
            No readable content for this URL.
          </p>
        )}

        <ReaderActions item={d} onDeleted={() => navigate("/library")} />
      </article>
    </>
  );
}
```

> Note: `dangerouslySetInnerHTML` is acceptable here because the backend already sanitizes article HTML via the readability extractor before persisting it. If that assumption changes in the future, swap to a frontend sanitizer (DOMPurify) — out of scope here.

- [x] **Step 2: Verify**

```bash
npm run typecheck
npm run test
```
Expected: PASS.

- [x] **Step 3: Commit**

```bash
git add web/src/routes/library.$id.tsx
git commit -m "feat(web/library): wire reader route with detail query, mark-as-read, actions"
```

---

## Группа 6 — Целостная проверка

### Task 26: Full test suite + lint + build

**Files:** (none — verification only)

- [x] **Step 1: Run lint, typecheck, tests, build**

Run from `web/`:
```bash
npm run lint
npm run typecheck
npm run test
npm run build
```
Expected: all PASS, no warnings.

- [x] **Step 2: If anything fails**

Fix in place. Common gotchas:
- ESLint may complain about unused `vi` imports — remove them.
- TanStack Query v5 type drift — if `pageParam` is `unknown`, cast at call site.
- `tailwind-merge` order — `cn(...)` already handles it.

Re-run until everything passes. Commit any fixes:
```bash
git add -A
git commit -m "chore(web): fix lint/typecheck/test issues after library wiring"
```

---

### Task 27: Manual smoke test against running backend

**Files:** (none — manual verification only)

- [ ] **Step 1: Start backend stack**

Run from repo root:
```bash
docker compose -f compose.dev.yaml up -d
```
Wait until `linktheca-server` is healthy.

- [ ] **Step 2: Start frontend dev server**

```bash
cd web && npm run dev
```
Open `http://localhost:5173`.

- [ ] **Step 3: Verify the golden path**

In the browser, perform the following and confirm each works:
- Log in. You see `/library` with the empty state «Nothing here yet».
- Click «+ Add link», paste a URL (e.g. `https://en.wikipedia.org/wiki/Information_retrieval`), Save.
  - Progress text cycles through three stages.
  - Dialog closes; toast «Saved to library» appears; the card appears in the grid.
- Click the card → reader opens.
  - Title, byline, host, reading-time row render.
  - Body prose renders with drop-cap on the first paragraph.
  - Scroll to the bottom — the small red progress bar at the top fills; state silently flips to «read» (refresh `/library`: card now shows «✓ read» stamp).
- In reader actions: click «Favorite» → reader shows it filled; back on `/library` → stamp shows.
- Click «Archive» → toast «Archived»; back on `/library` and switch filter to «Archived» → the card is there.
- Switch filter pills (All / Unread / Read / Archived) — URL updates; lists update.
- Toggle «Favorites only» — URL updates; only favorites visible.
- Reload the page on a filtered URL → filters are restored from the URL.
- In reader, click «Delete» → confirm → toast «Deleted» → redirected to `/library` → card no longer present.
- Try saving the same URL twice — error «This article is already in your library» appears inside the dialog.

- [ ] **Step 4: Verify mobile breakpoint**

In dev tools, set viewport to 375×800.
- Library grid collapses to 1 column.
- Filter pills wrap.
- Reader chrome stays readable; reading-progress bar still works.
- Add Link modal sits within the viewport with backdrop blur.

- [ ] **Step 5: Document anything off-spec**

If anything diverges from the spec (e.g. the prose-reader fonts look subtly different than the prototype, or the gradient hero feels too dark on a light-mode tab), note it in the PR description rather than chasing pixels in this plan — the foundation already established the visual tokens.

---

### Task 28: Final commit and merge readiness

**Files:** (none — wrap-up only)

- [ ] **Step 1: Final status check**

```bash
git status
git log --oneline -30
```
Expected: clean tree, all task commits visible.

- [ ] **Step 2: Push and open PR**

When ready:
```bash
git push origin <branch>
gh pr create --title "feat(web/library): library UI — list, add-link, reader" --body "$(cat <<'EOF'
## Summary
- Library list with state/favorite filters bound to URL search params
- Infinite pagination via TanStack Query (`useInfiniteQuery`)
- Add-link dialog with three-stage progress and error mapping (409/422/5xx)
- Reader view with prose-reader styling, drop-cap, reading-progress bar
- Mark-as-read auto-fires at 90% scroll on unread items
- Optimistic favorite toggle with rollback; archive + delete (with AlertDialog)

## Test plan
- [ ] `cd web && npm run lint && npm run typecheck && npm run test && npm run build`
- [ ] Manual smoke (Task 27): login → add link → reader → favorite → archive → delete

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Что вне scope этого плана (фиксируем)

| Тема | Куда |
|---|---|
| Sort options (oldest first) | Отложено до появления backend-параметра `order` |
| Note editor под reader'ом | Спек упоминает, но cost > value на текущей итерации; включим, если backend подтвердит `note`-flow (он уже есть в `PATCH`) — отдельная фича |
| Search в Library | Нет backend-endpoint'а — отдельный план |
| Tags | Backend не поддерживает — отдельный план |
| Bulk select / bulk actions | Не в спеке |
| Keyboard shortcuts (j/k, m, f) | Не в спеке |
| Reading-time из текста (если backend null) | Сейчас показываем «— read», достаточно |
| og:image / real hero images | Backend пока не извлекает; gradient deterministic placeholder |
| Undo-toast 5s для favorite/mark-read | Спек упоминает, но усложняет; начнём без undo — добавим если будут жалобы |
| Multi-tab refresh sync (BroadcastChannel) | Не в спеке |
