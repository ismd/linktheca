# Frontend: Foundation + Auth + Library — design

**Дата:** 2026-05-08
**Статус:** approved, готов к writing-plans
**Scope:** SPA на React, покрывающая auth (login/register) и Library (list, reader, add-link, edit, delete). Radar UI и Settings — отдельными планами после.

## Контекст

Backend для auth и Library уже готов и стабилен. Прототип `prototype/index.html` фиксирует визуальную систему (editorial/литературная эстетика, warm paper + vermillion accent, серif typography). Архитектурный спек `2026-04-10-architecture-design.md` (секция 7) задаёт стек на верхнем уровне; этот документ конкретизирует foundation, auth и Library до уровня implementation plan.

CLI-фаза 3a-3 намеренно пропущена — взаимодействие с Radar/Library будет через web-UI, а сейчас нужен сам web-UI.

## Решения, зафиксированные в brainstorming

| # | Вопрос | Решение |
|---|---|---|
| 1 | Scope | Foundation + Auth + Library. Radar и Settings — отдельные планы. |
| 2 | Локализация | English only, без i18n-библиотек. Копирайт хардкодим. |
| 3 | Типы API | Handwritten TS-типы + Zod-схемы. Никакого OpenAPI codegen на этом этапе. |
| 4 | Фазирование | Подход A: Foundation → Auth → Library как три последовательных мерджабельных куска. |
| 5 | Display font | Cormorant Garamond (вместо Fraunces). |
| 6 | Body font | Newsreader (без изменений). |
| 7 | Mono font | IBM Plex Mono (вместо JetBrains Mono — идеологические причины). |

## 1. Foundation: setup, структура, токены

### Стек

| Задача | Пакет |
|---|---|
| Build | Vite 5 |
| Язык | TypeScript strict |
| Стили | Tailwind CSS v4 (CSS-first config через `@theme`) |
| Компоненты | shadcn/ui (копии в `src/shared/ui/`) + Radix primitives |
| Icons | lucide-react |
| Шрифты | `@fontsource/cormorant-garamond` (400/500/600/700), `@fontsource-variable/newsreader`, `@fontsource/ibm-plex-mono` (400/500/600) |
| Роутер | React Router v7 (data mode, `BrowserRouter`) |
| Server state | TanStack Query v5 |
| Client state | Zustand (только для auth) |
| Формы | React Hook Form + Zod |
| Тесты | Vitest + RTL + MSW |
| Утилиты | clsx + tailwind-merge (`cn`), date-fns (relative time) |
| Toasts | sonner (через shadcn) |

**Намеренно не используем:** Next.js/Remix (нужен SPA), Redux (Zustand достаточно), MUI/Chakra (диктуют дизайн), styled-components/Emotion (Tailwind), Storybook, Playwright (E2E — отдельный этап).

### Структура `web/src/`

```
main.tsx, App.tsx
routes/
  __root.tsx          # AppShell layout route
  _public.tsx         # layout для login/register (no shell)
  login.tsx, register.tsx
  index.tsx           # → redirect /library
  library._index.tsx  # list
  library.$id.tsx     # reader
  settings.tsx        # заглушка с TODO
features/
  auth/      api.ts schemas.ts store.ts use-auth.ts components/
  library/   api.ts schemas.ts use-library.ts components/
shared/
  api/       client.ts errors.ts
  ui/        button.tsx card.tsx dialog.tsx input.tsx ...   # shadcn copies
  layout/    AppShell.tsx Sidebar.tsx Topbar.tsx PageHeader.tsx
             MobileDrawer.tsx PaperGrainOverlay.tsx
  hooks/     use-media-query.ts use-debounce.ts
  lib/       cn.ts time.ts
styles/
  globals.css   # Tailwind v4 + @theme tokens + utilities из прототипа
```

Принцип: всё, что относится к одной фиче — в одной директории. `shared/` — только то, что переиспользуется ≥2 фичами.

### Design tokens (`globals.css`)

```css
@import "tailwindcss";

@theme {
  --color-paper: #f3ece0;
  --color-paper-2: #ebe3d2;
  --color-paper-3: #e2d9c3;
  --color-ink: #1a1814;
  --color-ink-2: #2d2a24;
  --color-ink-3: #4a4438;
  --color-muted: #8a8275;
  --color-muted-2: #a69d8a;
  --color-rule: #d4cdbe;
  --color-rule-2: #e2dbc9;
  --color-vermillion: #c83832;
  --color-vermillion-dark: #9c241e;
  --color-ochre: #c89632;
  --color-sage: #6e8458;
  --color-plum: #6b3a4e;
  --font-display: "Cormorant Garamond", ui-serif, Georgia, serif;
  --font-body: "Newsreader", Georgia, serif;
  --font-mono: "IBM Plex Mono", ui-monospace, monospace;
}
```

Утилиты из прототипа (`.label-sc`, `.display-tight`, `.rule-dotted`, `.rule-double`, `.stamp`, `.paper-surface`, `.grain-overlay`, `.drop-cap`, `.prose-reader`, `.feed-card`, `.nav-item`, `.checkbox-custom`, `.toggle`, `.icon-btn`, `.tag-pill`, `.skeleton`, `.modal-backdrop`, `img-1..img-10` градиенты) переносим в `globals.css` как `@layer components`. Заменяем `font-family: 'Fraunces'` на `'Cormorant Garamond'`, `'JetBrains Mono'` на `'IBM Plex Mono'`. Drop-cap рисуем Cormorant SemiBold/Bold без variation-settings (Cormorant — статический шрифт).

### Dev workflow

- `cd web && npm run dev` → Vite на `:5173`.
- Vite proxy: `/api/*` → `http://localhost:8080`, rewrite `^/api → ""`. Backend остаётся неизменным, его роуты на `/auth`, `/library`.
- Frontend всегда зовёт `/api/auth/login`, `/api/library`, и т.д. Префикс `/api` фиксированный, `apiFetch` подставляет.

## 2. AppShell и responsive layout

### Breakpoints (Tailwind defaults)

| Имя | Ширина | Что меняется |
|---|---|---|
| `< md` (< 768) | phone | Single column. Hamburger открывает overlay-drawer. Topbar в одну строку. |
| `md` (≥ 768) | tablet | Library cards 2-up. Drawer всё ещё overlay. |
| `lg` (≥ 1024) | desktop | Sidebar pinned 280px. Drawer-режим выключен. Library cards 3-up. |
| `2xl` (≥ 1536) | wide | Reader column фиксирован 720px. Контент max-w-1280. |

### Layout-дерево

```
<RootRoute>                     # routes/__root.tsx
  <PaperGrainOverlay />         # fixed inset-0, pointer-events:none
  <AppShell>
    <Sidebar />                 # lg:fixed lg:w-[280px], <lg рендерится в drawer
    <Topbar />                  # sticky top-0 z-10, h-16
    <main>
      <Outlet />                # страница
    </main>
    <MobileDrawer />            # портал, открывается на <lg
  </AppShell>
</RootRoute>
```

### Sidebar

- На `lg+`: `position: fixed; left: 0; width: 280px; height: 100vh`. Содержит masthead (logo «Linktheca», Cormorant Italic), nav (Library / Radar disabled-stub / Settings), status footer.
- На `< lg`: тот же компонент, но рендерится внутри `MobileDrawer` (slide-in слева, backdrop с blur).
- `<NavLink>` из React Router — даёт active-state для красной полоски слева (`.nav-item.active::after`).

### MobileDrawer

- Реализован через `<Dialog>` из Radix (через shadcn) — focus-trap, ESC, click-backdrop-to-close, scroll-lock из коробки.
- Trigger — hamburger-кнопка в Topbar, видна только `< lg`.
- Закрывается на nav-click (через обработчик `useNavigate`).

### Topbar

- Sticky `top-0 z-10`, высота 64px, paper-2 фон с `border-b border-rule`.
- Слева: на `< lg` — hamburger; на `lg+` — пусто (logo уже в sidebar).
- Центр: расширяемый search input. **Search не реализован на бэкенде на этом этапе → элемент не рендерим**, помечаем TODO в коде.
- Справа: «+ Add Link» кнопка (открывает modal), user-menu (Cormorant initial → dropdown с logout).

### PageHeader pattern

Каждая страница импортирует `<PageHeader title="…" subtitle="…" actions={…} />`, который рендерит большой Cormorant-заголовок + small-caps подзаголовок + inline-actions. Это даёт единообразие без AppShell-привязки к конкретной странице.

### Public layout (`routes/_public.tsx`)

Без AppShell. Centered card на `paper-surface`, max-w-md, vertical-centered. Используется для `/login` и `/register`.

### Reader layout (внутри `library.$id.tsx`)

- Узкая колонка `max-w-[720px] mx-auto` для текста.
- `<ReadingProgress />` — fixed top, 2px vermillion bar, скейлится по `window.scrollY`.
- Drop-cap на первом параграфе через `.drop-cap` utility.

### Принципы

- Один `<Sidebar />` — рендерится и в drawer, и в pinned-режиме. Никакого условного рендеринга по media query для structural-выбора.
- `<MobileDrawer>` рендерится всегда, но открывается только когда state=open. Trigger (hamburger) невидим на `lg+` через Tailwind.
- Mobile-first CSS не делаем намеренно — прототип desktop-first, его подход сохраняем.

## 3. API client и auth state

### Хранение токенов

| Токен | Где |
|---|---|
| Access JWT (15 мин) | Zustand store, **только в памяти** |
| Refresh token (30 дней) | `localStorage` под ключом `linktheca.refresh` |

`localStorage` — compromise зафиксированный архспеком (Bearer-only, no cookies). Mitigation: строгий CSP, отсутствие сторонних скриптов, refresh-rotation на бэкенде отзывает украденный токен при первом легитимном использовании.

### Zustand store (`features/auth/store.ts`)

```ts
type AuthState = {
  accessToken: string | null;
  user: User | null;            // { id, email, displayName, isAdmin }
  status: 'bootstrapping' | 'authed' | 'anonymous';   // начинается с bootstrapping
  setSession: (access: string, user: User) => void;
  clearSession: () => void;
};
```

`status='bootstrapping'` — состояние при загрузке приложения, пока `apiFetch` не пытается обменять refresh из localStorage на access. До этого момента `<ProtectedRoute>` показывает full-page loading-state, а не редиректит на login (избегаем флэшей).

### API client (`shared/api/client.ts`)

Один экспорт — `apiFetch<T>(path, init?): Promise<T>`. Поведение:

1. Подставляет `/api` префикс и `Authorization: Bearer ${accessToken}` если токен есть.
2. Если ответ 401 и refresh-токен в localStorage — кладёт оригинальный запрос в очередь, дёргает `POST /auth/refresh`. **Один in-flight refresh** (Promise singleton): остальные 401-запросы ждут тот же Promise.
3. После успешного refresh — обновляет store, ретраит оригинальный запрос ровно один раз.
4. Если refresh упал → `clearSession()` + редирект на `/login`. Сохраняем `from`-location в state для возврата после login.
5. Не-401 ошибки нормализуем в `ApiError { status, code, message, details }` и пробрасываем.

```ts
class ApiError extends Error {
  constructor(public status: number, public code: string,
              message: string, public details?: unknown) { super(message); }
}
```

### Zod-парсинг ответов

Применяем точечно:

- `/auth/me`, `/auth/login`, `/auth/refresh` — всегда (рано ловим drift).
- Library list/item/content — Zod-парсинг только в dev/test через `import.meta.env.DEV`. В проде пропускаем (cost > benefit).

Утилита `parseInDev<T>(schema: ZodSchema<T>, data: unknown): T`.

### Bootstrap при загрузке

В `App.tsx` при mount:

```
status = 'bootstrapping'
if (localStorage.refresh)
   try POST /auth/refresh
   ok    → setSession(access, user); status = 'authed'
   fail  → clearSession(); status = 'anonymous'
else → status = 'anonymous'
```

`<ProtectedRoute>` рендерит `<FullPageSpinner>` пока `status === 'bootstrapping'`, иначе redirect на `/login` если `anonymous`, иначе `<Outlet />`.

### TanStack Query config

```ts
new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, err) =>
        err instanceof ApiError && err.status >= 500 ? failureCount < 2 : false,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
    mutations: { retry: false },
  },
});
```

`401` не ретраим Query'ем — refresh+retry уже сделан внутри `apiFetch`.

### Logout

`POST /auth/logout` с refresh-токеном в body → `clearSession()` → `localStorage.removeItem` → `queryClient.clear()` → redirect на `/login`. При сетевом fail всё равно clearSession локально — не блокируем юзера.

### Не входит в этот scope

«Stay logged in» чекбокс, multi-tab refresh sync через BroadcastChannel, idle-timeout, device list, password reset.

## 4. Routing, Auth screens, Library screens

### Route tree

```
/                         → redirect /library
/login                    public, _public layout
/register                 public, _public layout (если бэкенд вернёт 403 — показываем «Registration disabled»)
/library                  protected, AppShell, list
/library/:id              protected, AppShell + reader layout
/settings                 protected, заглушка
*                         404 page
```

`<ProtectedRoute>` оборачивает branch для `/library*` и `/settings`. На `/login` после успеха возвращаемся на `location.state.from ?? "/library"`.

### Login

- React Hook Form + Zod: email (валидный), password (≥1 char — длина проверяется бэкендом).
- Submit с inline-loading, disabled пока pending.
- Inline-error поля с `aria-describedby`.
- Top-of-form error: «Invalid email or password» при 401, «Service unavailable» при 5xx.
- Под формой `<Link to="/register">Create account →</Link>` — рендерим всегда, `/register` сам выдаёт 403/404 если регистрация выключена.

### Register

- Поля: email, display_name, password (≥10).
- Inline-validation password length с подсказкой.
- Submit → бэкенд возвращает access+refresh → setSession → redirect `/library`.
- 403 «Registration disabled» — full-page сообщение «New accounts are disabled on this instance».

### Visual для public layout

Centered card 400px на `paper-surface`, masthead «Linktheca» Cormorant Italic сверху, decorative `.rule-double` между masthead и формой.

### Library list

**Что грузим:** `useLibraryQuery({ state, favorite, page })`. Параметры — query string в URL (`useSearchParams`) для bookmarkable фильтра.

**Filters:**
- State pills: All / Unread / Read / Archived (single-select, default All).
- Favorite toggle: «Favorites only» on/off.
- Sort: «Recent first» / «Oldest first» (по `saved_at`).

**Card grid:**
- `< md` — 1 колонка.
- `md` — 2 колонки.
- `lg` — 3 колонки.
- Каждая карточка кликабельная (`<Link to=":id">`), показывает: hero strip (gradient `img-N` по `id % 10` пока нет real images), title (Cormorant), byline + reading-time, excerpt (3 lines clamp), state/favorite stamps, saved-at (relative, date-fns).

**Pagination:** `useInfiniteQuery` offset-based, страницы по 20, «Load more» button. Если бэкенд-контракт не offset/limit — адаптируем в `features/library/api.ts`, не везде.

**Empty state:** большой Cormorant «Nothing here yet» + small-caps «Save your first link →» + CTA-button открывает Add Link modal.

**Loading:** `.skeleton` карточки 6 штук на текущий breakpoint.

**Error:** `<ErrorPanel>` с retry, текст из `error.message`.

### Add Link modal

Trigger — кнопка в Topbar **и** empty-state CTA.

**Flow:**
1. Modal открывается, focus на URL input. Validation `z.string().url()`.
2. Submit → `POST /library { url }`. Бэкенд парсит контент **синхронно**, может занять 3–10 сек.
3. Pending UI: animated three-stage progress (декоративный, по таймеру — мы не получаем real progress events): «Fetching page…» → «Extracting content…» → «Saving to library…».
4. Success: invalidate `library-list` query, закрываем modal, toast «Saved to Library» с link на запись.
5. Error: показываем error в modal, не закрываем. 409 → «Already in library» с link на existing item; 422 → «Couldn't extract content from this URL».

**Реализация:** Radix Dialog → shadcn `<Dialog>`. Backdrop с blur, карточка с `.paper-surface`, max-w-lg.

### Reader view

**Что грузим параллельно:**
- `useLibraryItemQuery(id)` — meta (state, favorite, note, saved_at).
- `useLibraryContentQuery(id)` — text/html для prose.

Chrome рендерим как только meta готов, prose показывает skeleton пока content грузится.

**Layout:**
- `<ReadingProgress />` fixed top.
- Back-link «← Library» (Cormorant italic small).
- Article header: title (Cormorant Bold, display-size), byline + reading-time + saved-date (small-caps row), original-URL link.
- Hero figure: gradient placeholder или `<img>` если есть og:image.
- Body: `.prose-reader` с drop-cap на первом параграфе.
- Actions footer: row of icon-buttons — Mark read/unread, Favorite, Add note, Open original (external), Delete.

**Mark-as-read:** автоматически при scroll до 90% содержимого *И* `state==='unread'` → `PATCH /library/:id { state: 'read' }`. Один раз на загрузку страницы (флаг в local component state).

**Note:** клик «Add note» разворачивает textarea ниже content, autosave debounce 1s через `PATCH`.

**Delete:** Radix AlertDialog confirm → `DELETE /library/:id` → invalidate list → redirect `/library` с toast.

### Cross-cutting

- **Toasts:** sonner, top-right, paper-styled, 4s default. Save/delete confirmations и не-modal errors.
- **Confirmations:** только destructive (delete). Favorite/mark-read — без confirm, но с undo-toast 5s.
- **Optimistic updates:** для favorite-toggle и mark-as-read (`onMutate` snapshot → patch → rollback on error). Для delete — нет.
- **Form errors:** server-side `details: { field: message }` маппим в `setError(field, …)` React Hook Form.

## 5. Тестирование

### Что тестируем (по убыванию приоритета)

| Уровень | Что | Инструмент |
|---|---|---|
| Unit | API client (refresh-singleton, retry, error mapping); auth store transitions; relative-time helper | Vitest |
| Component | Формы (LoginForm, RegisterForm, AddLinkForm) — валидация/submit/errors; FilterPanel — change handlers; ProtectedRoute — redirect/spinner branches | Vitest + RTL |
| Integration | Library list → Add Link → Reader → Mark-as-read happy path с MSW | Vitest + RTL + MSW |

### Что не тестируем

- Презентационные компоненты (Card, Stamp, NavItem) — регрессии ловятся глазами.
- Visual regression / Storybook — overkill для одного разработчика.
- Real API integration — обязанность backend-тестов.
- E2E через Playwright — отдельный этап после MVP.

### MSW pattern

- Handlers: `features/<feature>/__mocks__/handlers.ts`.
- Общий `setupServer` в `src/test/setup.ts`, поднимается через `vitest.config.ts → setupFiles`.
- Handlers возвращают shapes из тех же Zod-схем, что парсит продовый код — один источник правды.

### npm scripts

```
npm run dev         # vite
npm run build       # tsc -b && vite build
npm run preview     # vite preview
npm run lint        # eslint --max-warnings 0
npm run typecheck   # tsc --noEmit
npm run test        # vitest run
npm run test:watch  # vitest
```

## 6. Сборка и deploy

### `web/Dockerfile` (multi-stage)

```dockerfile
# build stage
FROM node:24-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build           # → /app/dist

# runtime
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
```

### `web/nginx.conf`

```
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri /index.html;
    }

    location /api/ {
        proxy_pass http://backend:8080/;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    location = /index.html {
        add_header Cache-Control "no-store";
    }
}
```

`proxy_pass http://backend:8080/` со слэшем в конце — nginx стрипает `/api/` префикс. Совпадает с поведением Vite-прокси в dev.

### Compose

В **dev** web-сервис не поднимаем — Vite dev server живёт у разработчика на хосте, проксирует `/api` напрямую на `localhost:8080`. `compose.dev.yaml` остаётся как есть.

В **prod** добавляется отдельный файл `compose.prod.yaml` с web-сервисом:

```yaml
services:
  web:
    build: ./web
    ports: ["80:80"]
    depends_on: [backend]
```

### CI (GitHub Actions)

Добавляем job в существующий workflow:

```yaml
frontend:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: 24
        cache: npm
        cache-dependency-path: web/package-lock.json
    - run: npm ci
      working-directory: web
    - run: npm run lint && npm run typecheck && npm run test && npm run build
      working-directory: web
```

Запускается параллельно с backend-job.

### ESLint + Prettier

- ESLint: `@typescript-eslint`, `eslint-plugin-react-hooks`, `eslint-plugin-jsx-a11y`.
- Prettier: дефолты + `printWidth: 100`. Конфиг в корне `web/`.
- Pre-commit hook не делаем — CI ловит, лишний trip wire.

### Не входит в scope

Sentry / error monitoring, web vitals / analytics, bundle-size budget enforcement, PWA / Service Worker.

## 7. Что явно вне scope этого спека

- Radar UI (feed, topics, topic editor, threshold slider).
- Settings UI (профиль, изменение пароля, токены).
- Search в Library (нужен бэкенд-endpoint).
- Tags в Library.
- Mobile нативное приложение (другой репозиторий).
- E2E-тесты (Playwright) — после MVP.
- Visual regression, Storybook.
- i18n.

## Следующие шаги

После approval этого документа — переход к `superpowers:writing-plans` для трёх последовательных планов:

1. `2026-05-08-frontend-foundation.md` — setup, design tokens, AppShell, responsive layout, API client skeleton, route shell.
2. `2026-05-XX-frontend-auth.md` — login/register, refresh flow, ProtectedRoute, bootstrap.
3. `2026-05-XX-frontend-library.md` — list, filters, add-link, reader, edit, delete, optimistic updates, tests.

Каждая фаза мерджится самостоятельно. После всех трёх — приложение полностью покрывает Foundation+Auth+Library scope.