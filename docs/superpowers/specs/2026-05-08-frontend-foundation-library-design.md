# Frontend: Foundation + Auth + Library — design

**Date:** 2026-05-08
**Status:** approved, ready for writing-plans
**Scope:** a React SPA covering auth (login/register) and Library (list, reader, add-link, edit, delete). The Radar UI and Settings get their own plans afterwards.

## Context

The backend for auth and Library is finished and stable. The
`prototype/index.html` prototype fixes the visual system (an editorial, literary
aesthetic, warm paper plus a vermillion accent, serif typography). The
architecture spec `2026-04-10-architecture-design.md` (section 7) sets the stack
at a high level; this document takes the foundation, auth, and Library down to
implementation-plan detail.

The 3a-3 CLI phase is skipped deliberately — interaction with Radar and Library
will go through the web UI, and what we need now is the web UI itself.

## Decisions taken during brainstorming

| # | Question | Decision |
|---|---|---|
| 1 | Scope | Foundation + Auth + Library. Radar and Settings get their own plans. |
| 2 | Localization | English only, no i18n libraries. Copy is hardcoded. |
| 3 | API types | Handwritten TS types plus Zod schemas. No OpenAPI codegen at this stage. |
| 4 | Phasing | Approach A: Foundation → Auth → Library as three sequential, independently mergeable chunks. |
| 5 | Display font | Cormorant Garamond (instead of Fraunces). |
| 6 | Body font | Newsreader (unchanged). |
| 7 | Mono font | IBM Plex Mono (instead of JetBrains Mono — for ideological reasons). |

## 1. Foundation: setup, structure, tokens

### Stack

| Concern | Package |
|---|---|
| Build | Vite 5 |
| Language | TypeScript strict |
| Styling | Tailwind CSS v4 (CSS-first config through `@theme`) |
| Components | shadcn/ui (copies in `src/shared/ui/`) plus Radix primitives |
| Icons | lucide-react |
| Fonts | `@fontsource/cormorant-garamond` (400/500/600/700), `@fontsource-variable/newsreader`, `@fontsource/ibm-plex-mono` (400/500/600) |
| Router | React Router v7 (data mode, `BrowserRouter`) |
| Server state | TanStack Query v5 |
| Client state | Zustand (auth only) |
| Forms | React Hook Form + Zod |
| Tests | Vitest + RTL + MSW |
| Utilities | clsx + tailwind-merge (`cn`), date-fns (relative time) |
| Toasts | sonner (through shadcn) |

**Deliberately not used:** Next.js/Remix (we need an SPA), Redux (Zustand is
enough), MUI/Chakra (they dictate the design), styled-components/Emotion
(Tailwind), Storybook, Playwright (E2E is a separate stage).

### The `web/src/` structure

```
main.tsx, App.tsx
routes/
  __root.tsx          # AppShell layout route
  _public.tsx         # layout for login/register (no shell)
  login.tsx, register.tsx
  index.tsx           # → redirect /library
  library._index.tsx  # list
  library.$id.tsx     # reader
  settings.tsx        # a stub with a TODO
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
  globals.css   # Tailwind v4 + @theme tokens + utilities from the prototype
```

The principle: everything belonging to one feature lives in one directory.
`shared/` holds only what is reused by two or more features.

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

The prototype's utilities (`.label-sc`, `.display-tight`, `.rule-dotted`,
`.rule-double`, `.stamp`, `.paper-surface`, `.grain-overlay`, `.drop-cap`,
`.prose-reader`, `.feed-card`, `.nav-item`, `.checkbox-custom`, `.toggle`,
`.icon-btn`, `.tag-pill`, `.skeleton`, `.modal-backdrop`, and the `img-1..img-10`
gradients) move into `globals.css` as `@layer components`. We replace
`font-family: 'Fraunces'` with `'Cormorant Garamond'` and `'JetBrains Mono'` with
`'IBM Plex Mono'`. The drop cap is drawn in Cormorant SemiBold/Bold without
variation settings (Cormorant is a static font).

### Dev workflow

- `cd web && npm run dev` → Vite on `:5173`.
- Vite proxy: `/api/*` → `http://localhost:8080`, rewriting `^/api → ""`. The
  backend is untouched; its routes stay at `/auth` and `/library`.
- The frontend always calls `/api/auth/login`, `/api/library`, and so on. The
  `/api` prefix is fixed and `apiFetch` supplies it.

## 2. AppShell and responsive layout

### Breakpoints (Tailwind defaults)

| Name | Width | What changes |
|---|---|---|
| `< md` (< 768) | phone | Single column. The hamburger opens an overlay drawer. Topbar on one line. |
| `md` (≥ 768) | tablet | Library cards 2-up. The drawer is still an overlay. |
| `lg` (≥ 1024) | desktop | Sidebar pinned at 280px. Drawer mode off. Library cards 3-up. |
| `2xl` (≥ 1536) | wide | The reader column is fixed at 720px. Content max-w-1280. |

### The layout tree

```
<RootRoute>                     # routes/__root.tsx
  <PaperGrainOverlay />         # fixed inset-0, pointer-events:none
  <AppShell>
    <Sidebar />                 # lg:fixed lg:w-[280px]; below lg it renders inside the drawer
    <Topbar />                  # sticky top-0 z-10, h-16
    <main>
      <Outlet />                # the page
    </main>
    <MobileDrawer />            # a portal, opens below lg
  </AppShell>
</RootRoute>
```

### Sidebar

- At `lg+`: `position: fixed; left: 0; width: 280px; height: 100vh`. It holds the
  masthead (the "Linktheca" logo, Cormorant Italic), the nav (Library / a
  disabled Radar stub / Settings), and a status footer.
- Below `lg`: the same component, rendered inside `MobileDrawer` (sliding in from
  the left, with a blurred backdrop).
- `<NavLink>` from React Router supplies the active state for the red bar on the
  left (`.nav-item.active::after`).

### MobileDrawer

- Built on Radix's `<Dialog>` (through shadcn) — focus trap, ESC,
  click-backdrop-to-close, and scroll lock out of the box.
- The trigger is the hamburger button in the Topbar, visible only below `lg`.
- It closes on a nav click (through a `useNavigate` handler).

### Topbar

- Sticky `top-0 z-10`, 64px tall, a paper-2 background with
  `border-b border-rule`.
- On the left: below `lg`, the hamburger; at `lg+`, nothing (the logo is already
  in the sidebar).
- In the centre: an expanding search input. **Search is not implemented on the
  backend at this stage, so we do not render the element**; a TODO marks it in
  the code.
- On the right: the "+ Add Link" button (which opens the modal) and the user menu
  (a Cormorant initial opening a dropdown with logout).

### The PageHeader pattern

Every page imports `<PageHeader title="…" subtitle="…" actions={…} />`, which
renders a large Cormorant heading plus a small-caps subtitle plus inline
actions. That gives uniformity without tying the AppShell to any particular
page.

### Public layout (`routes/_public.tsx`)

No AppShell. A centred card on `paper-surface`, max-w-md, vertically centred.
Used for `/login` and `/register`.

### Reader layout (inside `library.$id.tsx`)

- A narrow `max-w-[720px] mx-auto` column for the text.
- `<ReadingProgress />` — fixed at the top, a 2px vermillion bar, scaled by
  `window.scrollY`.
- A drop cap on the first paragraph through the `.drop-cap` utility.

### Principles

- One `<Sidebar />` — it renders both in the drawer and in pinned mode. No
  conditional rendering by media query for structural choices.
- `<MobileDrawer>` always renders but only opens when state is open. The trigger
  (the hamburger) is invisible at `lg+` through Tailwind.
- We deliberately do not write mobile-first CSS — the prototype is
  desktop-first, and we keep its approach.

## 3. The API client and auth state

### Token storage

| Token | Where |
|---|---|
| Access JWT (15 min) | The Zustand store, **in memory only** |
| Refresh token (30 days) | `localStorage` under the key `linktheca.refresh` |

`localStorage` is the compromise recorded in the architecture spec (Bearer-only,
no cookies). Mitigation: a strict CSP, no third-party scripts, and refresh
rotation on the backend which revokes a stolen token at the first legitimate
use.

### The Zustand store (`features/auth/store.ts`)

```ts
type AuthState = {
  accessToken: string | null;
  user: User | null;            // { id, email, displayName, isAdmin }
  status: 'bootstrapping' | 'authed' | 'anonymous';   // starts at bootstrapping
  setSession: (access: string, user: User) => void;
  clearSession: () => void;
};
```

`status='bootstrapping'` is the state while the app loads and before `apiFetch`
has tried to exchange the refresh token from localStorage for an access token.
Until then `<ProtectedRoute>` shows a full-page loading state rather than
redirecting to login (this avoids flashes).

### The API client (`shared/api/client.ts`)

One export — `apiFetch<T>(path, init?): Promise<T>`. Its behaviour:

1. Prepends the `/api` prefix and adds `Authorization: Bearer ${accessToken}` if
   there is a token.
2. If the response is 401 and there is a refresh token in localStorage, it queues
   the original request and calls `POST /auth/refresh`. **One in-flight refresh**
   (a Promise singleton): every other 401 request waits on that same Promise.
3. After a successful refresh it updates the store and retries the original
   request exactly once.
4. If the refresh fails → `clearSession()` plus a redirect to `/login`. We keep
   the `from` location in state so we can return there after login.
5. Non-401 errors are normalized into `ApiError { status, code, message, details }`
   and rethrown.

```ts
class ApiError extends Error {
  constructor(public status: number, public code: string,
              message: string, public details?: unknown) { super(message); }
}
```

### Zod parsing of responses

Applied selectively:

- `/auth/me`, `/auth/login`, `/auth/refresh` — always (catching drift early).
- Library list/item/content — Zod parsing only in dev and test, through
  `import.meta.env.DEV`. Skipped in production (cost > benefit).

The `parseInDev<T>(schema: ZodSchema<T>, data: unknown): T` utility.

### Bootstrap on load

In `App.tsx` on mount:

```
status = 'bootstrapping'
if (localStorage.refresh)
   try POST /auth/refresh
   ok    → setSession(access, user); status = 'authed'
   fail  → clearSession(); status = 'anonymous'
else → status = 'anonymous'
```

`<ProtectedRoute>` renders `<FullPageSpinner>` while `status === 'bootstrapping'`,
otherwise redirects to `/login` if `anonymous`, otherwise renders `<Outlet />`.

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

We do not retry `401` at the Query level — the refresh-and-retry already happened
inside `apiFetch`.

### Logout

`POST /auth/logout` with the refresh token in the body → `clearSession()` →
`localStorage.removeItem` → `queryClient.clear()` → redirect to `/login`. On a
network failure we clear the session locally anyway — we do not block the user.

### Not in this scope

A "Stay logged in" checkbox, multi-tab refresh sync through BroadcastChannel, an
idle timeout, a device list, password reset.

## 4. Routing, auth screens, Library screens

### The route tree

```
/                         → redirect /library
/login                    public, _public layout
/register                 public, _public layout (if the backend returns 403 we show "Registration disabled")
/library                  protected, AppShell, list
/library/:id              protected, AppShell + reader layout
/settings                 protected, a stub
*                         404 page
```

`<ProtectedRoute>` wraps the branch for `/library*` and `/settings`. On `/login`,
after success we return to `location.state.from ?? "/library"`.

### Login

- React Hook Form + Zod: email (valid), password (≥1 char — the length is checked
  by the backend).
- Submit with inline loading, disabled while pending.
- Inline field errors with `aria-describedby`.
- A top-of-form error: "Invalid email or password" on a 401, "Service
  unavailable" on a 5xx.
- Below the form, `<Link to="/register">Create account →</Link>` — always
  rendered; `/register` itself returns 403/404 when registration is off.

### Register

- Fields: email, display_name, password (≥10).
- Inline validation of password length with a hint.
- Submit → the backend returns access plus refresh → setSession → redirect to
  `/library`.
- A 403 "Registration disabled" → a full-page message, "New accounts are disabled
  on this instance".

### Visuals for the public layout

A centred 400px card on `paper-surface`, the "Linktheca" masthead in Cormorant
Italic at the top, and a decorative `.rule-double` between the masthead and the
form.

### Library list

**What we load:** `useLibraryQuery({ state, favorite, page })`. The parameters
live in the URL query string (`useSearchParams`) so filters are bookmarkable.

**Filters:**
- State pills: All / Unread / Read / Archived (single-select, All by default).
- A favorite toggle: "Favorites only" on/off.
- Sort: "Recent first" / "Oldest first" (by `saved_at`).

**Card grid:**
- `< md` — 1 column.
- `md` — 2 columns.
- `lg` — 3 columns.
- Every card is clickable (`<Link to=":id">`) and shows: a hero strip (an
  `img-N` gradient by `id % 10` while there are no real images), the title
  (Cormorant), a byline plus reading time, an excerpt (clamped to 3 lines),
  state/favorite stamps, and saved-at (relative, date-fns).

**Pagination:** an offset-based `useInfiniteQuery`, 20 per page, with a "Load
more" button. If the backend contract is not offset/limit, we adapt inside
`features/library/api.ts` and nowhere else.

**Empty state:** a large Cormorant "Nothing here yet" plus a small-caps "Save
your first link →" plus a CTA button that opens the Add Link modal.

**Loading:** six `.skeleton` cards at the current breakpoint.

**Error:** an `<ErrorPanel>` with retry and the text from `error.message`.

### The Add Link modal

Triggered from the Topbar button **and** from the empty-state CTA.

**Flow:**
1. The modal opens with focus on the URL input. Validation is
   `z.string().url()`.
2. Submit → `POST /library { url }`. The backend parses the content
   **synchronously**, which can take 3–10 seconds.
3. Pending UI: an animated three-stage progress indicator (decorative, on a
   timer — we get no real progress events): "Fetching page…" → "Extracting
   content…" → "Saving to library…".
4. Success: invalidate the `library-list` query, close the modal, and show a
   "Saved to Library" toast linking to the entry.
5. Error: show it inside the modal and keep the modal open. A 409 → "Already in
   library" with a link to the existing item; a 422 → "Couldn't extract content
   from this URL".

**Implementation:** Radix Dialog through shadcn's `<Dialog>`. A blurred backdrop,
a `.paper-surface` card, max-w-lg.

### Reader view

**What we load in parallel:**
- `useLibraryItemQuery(id)` — the metadata (state, favorite, note, saved_at).
- `useLibraryContentQuery(id)` — the text/html for the prose.

The chrome renders as soon as the metadata is ready; the prose shows a skeleton
while the content loads.

**Layout:**
- `<ReadingProgress />` fixed at the top.
- A "← Library" back link (Cormorant italic, small).
- The article header: the title (Cormorant Bold, display size), a byline plus
  reading time plus the saved date (a small-caps row), and a link to the original
  URL.
- A hero figure: a gradient placeholder, or an `<img>` when there is an og:image.
- The body: `.prose-reader` with a drop cap on the first paragraph.
- An actions footer: a row of icon buttons — Mark read/unread, Favorite, Add
  note, Open original (external), Delete.

**Mark as read:** automatic when the user scrolls to 90% of the content *and*
`state==='unread'` → `PATCH /library/:id { state: 'read' }`. Once per page load
(tracked by a flag in local component state).

**Note:** clicking "Add note" expands a textarea below the content, autosaved on
a 1s debounce through `PATCH`.

**Delete:** a Radix AlertDialog confirmation → `DELETE /library/:id` →
invalidate the list → redirect to `/library` with a toast.

### Cross-cutting concerns

- **Toasts:** sonner, top right, paper-styled, 4s by default. For save and delete
  confirmations and for non-modal errors.
- **Confirmations:** destructive actions only (delete). Favorite and mark-read
  have no confirmation but do get a 5s undo toast.
- **Optimistic updates:** for the favorite toggle and mark-as-read (`onMutate`
  snapshot → patch → rollback on error). Not for delete.
- **Form errors:** the server's `details: { field: message }` maps to React Hook
  Form's `setError(field, …)`.

## 5. Testing

### What we test (in descending priority)

| Level | What | Tool |
|---|---|---|
| Unit | The API client (the refresh singleton, retry, error mapping); auth store transitions; the relative-time helper | Vitest |
| Component | Forms (LoginForm, RegisterForm, AddLinkForm) — validation/submit/errors; FilterPanel — change handlers; ProtectedRoute — the redirect and spinner branches | Vitest + RTL |
| Integration | The Library list → Add Link → Reader → Mark-as-read happy path with MSW | Vitest + RTL + MSW |

### What we do not test

- Presentational components (Card, Stamp, NavItem) — regressions are caught by
  eye.
- Visual regression / Storybook — overkill for one developer.
- Real API integration — that is the backend tests' job.
- E2E through Playwright — a separate stage after the MVP.

### The MSW pattern

- Handlers: `features/<feature>/__mocks__/handlers.ts`.
- A shared `setupServer` in `src/test/setup.ts`, wired through
  `vitest.config.ts → setupFiles`.
- Handlers return shapes from the same Zod schemas the production code parses —
  one source of truth.

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

## 6. Build and deploy

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

`proxy_pass http://backend:8080/` with the trailing slash makes nginx strip the
`/api/` prefix. That matches the Vite proxy's behaviour in dev.

### Compose

In **dev** we do not bring up a web service — the Vite dev server runs on the
developer's host and proxies `/api` straight to `localhost:8080`.
`compose.dev.yaml` stays as it is.

In **prod** a separate `compose.prod.yaml` adds the web service:

```yaml
services:
  web:
    build: ./web
    ports: ["80:80"]
    depends_on: [backend]
```

### CI (GitHub Actions)

We add a job to the existing workflow:

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

It runs in parallel with the backend job.

### ESLint + Prettier

- ESLint: `@typescript-eslint`, `eslint-plugin-react-hooks`,
  `eslint-plugin-jsx-a11y`.
- Prettier: the defaults plus `printWidth: 100`. The config lives at the root of
  `web/`.
- No pre-commit hook — CI catches it, and one less trip wire.

### Not in scope

Sentry / error monitoring, web vitals / analytics, bundle-size budget
enforcement, PWA / Service Worker.

## 7. Explicitly out of this spec's scope

- The Radar UI (feed, topics, topic editor, threshold slider).
- The Settings UI (profile, password change, tokens).
- Search in Library (it needs a backend endpoint).
- Tags in Library.
- The native mobile app (a different repository).
- E2E tests (Playwright) — after the MVP.
- Visual regression, Storybook.
- i18n.

## Next steps

After this document is approved — move to `superpowers:writing-plans` for three
sequential plans:

1. `2026-05-08-frontend-foundation.md` — setup, design tokens, AppShell,
   responsive layout, the API client skeleton, the route shell.
2. `2026-05-XX-frontend-auth.md` — login/register, the refresh flow,
   ProtectedRoute, bootstrap.
3. `2026-05-XX-frontend-library.md` — list, filters, add-link, reader, edit,
   delete, optimistic updates, tests.

Each phase merges on its own. After all three, the application fully covers the
Foundation + Auth + Library scope.
