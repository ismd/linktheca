# Settings Screen — Design (read-only, frontend-only)

**Date:** 2026-06-04
**Status:** approved, ready for an implementation plan
**Related to:** the `prototype/index.html` prototype (Settings section), `2026-05-18-radar-frontend-design.md` (the feature-module pattern, `useRadarStatusQuery`)

---

## Goal

Replace the `web/src/routes/settings.tsx` stub ("Coming soon") with a real
**read-only** screen. That closes the last remaining stub in the sidebar
(`/settings`, item "03") and completes the MVP UI surface.

**No backend changes.** The screen consumes only what the API already serves
today: `useAuthStore` (populated from `GET /auth/me`) and `GET /radar/status`.

## Non-goals (explicitly out of scope)

These items from the prototype are deliberately **not** built in this iteration:

- Editing display name / email / password (no backend endpoints; a separate
  "Editable Account" feature).
- API tokens.
- Appearance settings (theme, drop caps, paper grain, progress bar) — no
  persistence and no effect on the reader.
- Notifications (browser / email digest / ntfy) — architecturally out of MVP
  scope.
- Monitoring / Parsing config (refresh interval, embedding model, threshold, max
  sources, user-agent) — env-level and/or deferred features (per-topic threshold
  is its own deferred feature).
- Instance metrics: hostname, uptime, database size, storage, export archive,
  view logs — no data source.

If an editable Account is wanted later, that is a new brainstorming session with
its own spec and backend work.

## Decisions taken during brainstorming

| Question | Decision |
|---|---|
| Iteration scope | Read-only, frontend-only. Zero new backend. |
| Set of sections | Two: **Account** and **About**. |
| Structure | One column, section cards stacked. No left section-nav (redundant for two sections). Mirrors the Library/Radar pattern. |
| Logout | Not duplicated — it already lives in `UserMenu` (header). Account is read-only, with no actions. |
| Source of the version | A static constant (package.json `version` is not supported). Extracted into `shared/version.ts`. |

---

## Architecture

### Placement

A light feature module at `web/src/features/settings/` — **without**
`api.ts`/`schemas.ts`/hooks of its own, because it has no data of its own:
Account comes from `useAuthStore` (`features/auth`), and Radar status from
`useRadarStatusQuery` (`features/radar`). That is an acceptable cross-feature
consume (Settings is a cross-cutting consumer, not a breach of the
Library/Radar boundary).

### Files

| Path | Change |
|---|---|
| `web/src/shared/version.ts` | create — `export const APP_VERSION = "0.1.0";` |
| `web/src/features/settings/components/SettingRow.tsx` | create — a presentational `label · value` row |
| `web/src/features/settings/components/AccountSection.tsx` | create |
| `web/src/features/settings/components/AccountSection.test.tsx` | create |
| `web/src/features/settings/components/AboutSection.tsx` | create |
| `web/src/features/settings/components/AboutSection.test.tsx` | create |
| `web/src/routes/settings.tsx` | rewrite — header + `<AccountSection />` + `<AboutSection />` |
| `web/src/routes/settings.test.tsx` | create — renders both section headings |
| `web/src/shared/layout/Sidebar.tsx` | modify — replace the hardcoded `v0.1.0` with `APP_VERSION` |

---

## Components

### `SettingRow`

Presentational, stateless. Draws a row: a small uppercase label on the left or
above, and a `text-ink` value. Mirrors `settingRow` from the prototype, minus
the editable buttons (everything is read-only).

```
Props: { label: string; value: ReactNode }
```

Used in both Account (Email, Role) and About (Version, Mode, Radar).

### `AccountSection`

```tsx
const user = useAuthStore((s) => s.user);
if (!user) return null; // ProtectedRoute guarantees authed, but be defensive
const initial = user.displayName.charAt(0).toUpperCase() || "·";
const role = user.isAdmin ? "Administrator" : "Member";
```

Renders:
- The initial avatar: a `w-16 h-16 bg-ink text-paper font-mono` square (the same
  pattern as in `UserMenu`).
- The `display-tight` name (`user.displayName`).
- `SettingRow` Email = `user.email`.
- `SettingRow` Role = `role`.

No Edit/Change buttons (editing is out of scope — we do not show controls that
do nothing).

### `AboutSection`

```tsx
const status = useRadarStatusQuery();
```

Renders three `SettingRow`s:
- **Version** = `v{APP_VERSION}` (→ `v0.1.0`).
- **Mode** = `self-hosted` (static).
- **Radar** = a string derived from the query state (below).

#### Radar state mapping (the only dynamic thing on the screen)

| Query state | Radar row value |
|---|---|
| `isLoading` | `Checking…` |
| `isSuccess` | `fmtSweep(data.lastSweepAt)` → `Last sweep · 2h ago` or `Awaiting first sweep` |
| `isError` && `error instanceof ApiError` && `error.code === "radar_disabled"` | `Disabled` |
| any other error | `Unavailable` |

`fmtSweep` is imported from `@/features/radar/time`. The `radar_disabled` check
mirrors `routes/radar._index.tsx` (`error.code === "radar_disabled"`). `ApiError`
carries `status` and `code` fields.

### `routes/settings.tsx`

Rewritten from the stub into:

```tsx
<div>
  <PageHeader title="Settings" subtitle="This instance and your account." />
  <AccountSection />
  <AboutSection />
</div>
```

The subtitle `This instance and your account.` is a concrete default; an
equivalent in the same editorial tone is acceptable.

Each section is a `bg-paper-2 border border-rule p-6 md:p-8` card with a
`display-tight` heading and an italic subheading (the prototype's
`settingsSection` idiom, and the one already used on the Library/Radar screens).
The exact heading markup (`PageHeader` vs a custom wonky header like the
prototype's) is left to the implementation, within the existing layout pattern.

---

## Data flow

```
AccountSection ── useAuthStore (filled by auth bootstrap/login) ── displayName, email, isAdmin
AboutSection ──┬─ APP_VERSION (shared/version.ts) ─────────────── Version, Mode
               └─ useRadarStatusQuery → GET /radar/status ──────── Radar row
Sidebar ─────── APP_VERSION (shared/version.ts) ───────────────── footer line
```

## Error handling

- **Account:** the `!user → null` guard. On a protected route there is always a
  user; the guard covers the race during sign-out.
- **About / Radar:** the row never throws — every query state (loading / success
  / disabled / other-error) maps to a string. The screen renders fully even with
  Radar switched off (consistent with the sidebar keeping Radar visible under
  `radar_disabled`).

## Testing

Vitest + Testing Library + MSW, modelled on the radar tests: a `QueryClient`
wrapper with `retry: false`, `useAuthStore.getState().setSession(...)` in
`beforeEach`, `server.use(http.get(...))` for `/api/radar/status`.

- **`AccountSection.test.tsx`**
  - an authed session (`isAdmin: false`) → `displayName`, `email`, `Member` are
    visible.
  - an authed session (`isAdmin: true`) → `Administrator` is visible.
- **`AboutSection.test.tsx`**
  - `/radar/status` → `{ last_sweep_at: "<iso>" }` → `v0.1.0`, `self-hosted`, and
    the sweep text (`Last sweep · …`) are visible.
  - `/radar/status` → an error with `code: "radar_disabled"` → `Disabled` is
    visible.
- **`settings.test.tsx`**
  - the route renders both section headings: `Account` and `About`.

## Style

The project's editorial idiom: `bg-paper-2 border border-rule` cards,
`display-tight` section headings, italic subheadings, `label-sc`/`font-mono` for
labels and values — as in the prototype (`settingsSection`, `settingRow`) and on
the finished Library/Radar screens. No new design primitives.

---

## Spec coverage

| What | Where it is covered |
|---|---|
| `shared/version.ts` plus the Sidebar refactor | Architecture → Files |
| `SettingRow` | Components |
| Account (identity, role, read-only) | Components → AccountSection |
| About (version, mode, radar) | Components → AboutSection |
| Radar states (loading/success/disabled/error) | Radar state mapping |
| Route composition | routes/settings.tsx |
| Tests | Testing |
| Explicit non-goals | Non-goals |

The backend is untouched. Every data source exists today.
