# Personal sources and the Admin section — design

**Status:** approved, ready for implementation plan.
**Date:** 2026-08-24.

## Problem

`radar_feeds` is an instance-level table with no owner. Every user sees the
whole catalog, and only an admin can add, edit, or delete a feed
(`POST|PATCH|DELETE /radar/feeds` behind `RequireAdmin`,
`internal/server/server.go:163-168`). The `/radar/sources` screen shows one flat
list with the admin's Edit/Delete buttons wired into its rows.

That leaves two defects:

1. **A user cannot add their own source.** Following a blog nobody else cares
   about means asking an admin to put it in the shared catalog — where it then
   clutters the catalog for everyone.
2. **Curating the catalog is mixed with consuming it.** One screen answers both
   "what am I subscribed to" and "what does this instance fetch at all". Those
   are different jobs, with different audiences and different frequencies.

We split them into two objects of management: **personal sources** (a user adds
and manages them, visible only to them) and the **global catalog** (curated by
an admin, available to everyone to subscribe to). The catalog moves into a new
top-level **Admin** section, which will later gain user management.

This closes `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` and
explicitly revises decision #1 of
`docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md` ("Admin actions
are embedded in the same rows; there is no separate admin area").

## Scope

**In scope:** an owner column on `radar_feeds`; personal feeds — create, edit,
pause, delete, auto-subscribe on creation; deduplication against the global
catalog; a quota on personal feeds via env; an Admin item in the sidebar behind
a role gate; an `/admin/sources` screen managing the global catalog; splitting
`/radar/sources` into two sections.

**Out of scope:** user management inside Admin (the section's next step);
promoting a personal feed into the catalog and demoting it back; editing quotas
from the UI and an `instance_config` table; how admins are appointed (still
"first registered user", `internal/auth/service.go:61-65`); deduplicating crawls
of the same URL across owners; skipping feeds nobody subscribes to
(`ListDueFeeds` is left alone).

## Decisions

1. **Ownership lives in an `owner_user_id` column.** `NULL` means a global feed,
   anything else means that user's personal feed. One visibility rule holds
   everywhere in the code: `owner_user_id IS NULL OR owner_user_id = :caller`.
2. **URL uniqueness is partial.** Global feeds are unique by `url`, personal
   ones by `(url, owner_user_id)`. Two different users adding the same URL get
   two rows, and the feed is fetched twice. That is the deliberate price: the
   alternative is sharing the first adder's personal feed with the second, which
   blurs ownership and leaks privacy. On a self-hosted instance with a handful
   of accounts a duplicate fetch is the cheaper cost.
3. **Deduplicate against the global catalog.** When a user adds a URL that is
   already in the catalog, no new row is created — they are subscribed to the
   existing feed. Without this, one popular feed would sit in the database once
   per account.
4. **The route namespace carries the scope.** `/radar/feeds` only writes the
   caller's personal feeds; `/admin/radar/feeds` only writes global ones. No
   `if isAdmin` branch inside a shared handler: a handler physically cannot pick
   the wrong scope, because it calls different service methods.
5. **Outside your scope means 404, not 403.** Another account's personal feed
   must look nonexistent; a 403 would confirm that a feed with that id exists.
6. **Quota via `LINKTHECA_RADAR_MAX_USER_FEEDS`, default 20.** An open write
   endpoint with no ceiling is a DoS vector for the crawler and for TEI — the
   very reason `POST /radar/feeds` sat behind `RequireAdmin` in the first place.
   `fetch_interval_seconds` already has a floor of 300 seconds
   (`internal/radar/service.go:113-117`), so personal feeds need no separate
   lower bound.
7. **Admin sees the global catalog only.** Users' personal feeds are not shown
   to it: the instance is self-hosted, but "the admin reads everyone's
   subscriptions" is exactly the privacy we are splitting ownership to protect.

## Database schema

New migration `migrations/014_radar_feeds_owner.sql`:

```sql
-- +goose Up
ALTER TABLE radar_feeds
  ADD COLUMN owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE radar_feeds DROP CONSTRAINT radar_feeds_url_key;

CREATE UNIQUE INDEX radar_feeds_global_url_idx
  ON radar_feeds (url) WHERE owner_user_id IS NULL;
CREATE UNIQUE INDEX radar_feeds_owner_url_idx
  ON radar_feeds (url, owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX radar_feeds_owner_idx
  ON radar_feeds (owner_user_id) WHERE owner_user_id IS NOT NULL;

-- +goose Down
-- Fails if personal feeds share a URL with each other or with a catalog feed:
-- the restored constraint is stricter than the partial indexes it replaces.
DROP INDEX radar_feeds_owner_idx;
DROP INDEX radar_feeds_owner_url_idx;
DROP INDEX radar_feeds_global_url_idx;
ALTER TABLE radar_feeds ADD CONSTRAINT radar_feeds_url_key UNIQUE (url);
ALTER TABLE radar_feeds DROP COLUMN owner_user_id;
```

Existing rows stay global; no backfill is needed.

`ON DELETE CASCADE` rather than the deferred spec's `SET NULL`: deleting a user
must take their feeds along with the findings and matches. `SET NULL` would
silently promote a deleted user's personal feeds into the catalog, and everyone
else would start receiving matches from sources they never chose.

Nothing else in the schema changes. The
`radar_feeds → radar_findings → radar_topic_matches` chain is already entirely
`ON DELETE CASCADE`.

## API

| Method | Path | Access | Behaviour |
|---|---|---|---|
| `GET` | `/radar/feeds` | user | Visible feeds: global plus the caller's own. Sorted by `(owner_user_id IS NULL), lower(coalesce(title, url))` — personal first. Pagination unchanged (`limit` ≤ 100, default 50). |
| `POST` | `/radar/feeds` | user | Creates a **personal** feed and subscribes the caller to it. 201. If the URL is already in the global catalog, no row is created, a subscription is made, 200. A duplicate of one's own → 409 `duplicate`. Quota exhausted → 409 `quota_exceeded`. |
| `PATCH` | `/radar/feeds/{id}` | user | Own personal feeds only. Fields unchanged: `title`, `fetch_interval_seconds`, `is_active`. A global or foreign feed → 404. |
| `DELETE` | `/radar/feeds/{id}` | user | Own personal feeds only. 204, cascade. A global or foreign feed → 404. |
| `POST` | `/radar/subscriptions` | user | Unchanged from the outside; internally it now rejects another account's personal feed → 404 `not_found`. |
| `DELETE` | `/radar/subscriptions/{feedId}` | user | Unchanged. Idempotent. |
| `GET` | `/admin/radar/feeds` | admin | Global rows only. |
| `POST` | `/admin/radar/feeds` | admin | Creates a global feed. Duplicate → 409 `duplicate`. No auto-subscription — neither for the admin nor for existing users. |
| `PATCH` | `/admin/radar/feeds/{id}` | admin | Global rows only. A personal feed → 404. |
| `DELETE` | `/admin/radar/feeds/{id}` | admin | Global rows only. 204, cascade. |

With `LINKTHECA_RADAR_ENABLED=false`, `/admin/radar/*` answers `radar_disabled`
just as `/radar/*` does.

### DTO changes

`FeedListItem` (`internal/radar/types.go:177`) gains `is_own bool`. The
`owner_user_id` itself is not exposed — the frontend only needs "mine / not
mine", and an owner id in the response would leak without buying anything.

`RadarStatus` (`:235`) gains `max_user_feeds int` so the Sources screen can show
`4 / 20` without a separate endpoint.

A new `ErrQuotaExceeded` sentinel and a branch in `writeRadarError`
(`http.go:71`): 409 with code `quota_exceeded`. Same status as `duplicate`, but
a different code — the frontend distinguishes them by `ApiError.code`.

## Backend

### Store (`internal/radar/store.go`)

Every feed query gains an ownership filter.

`ListFeeds` (`:572`) — a `WHERE` built from the new `Scope`, applied to **both
the page and the `count(*)`**: today the total counts the whole table, which
would be inflated once personal feeds exist.

`Subscribe` (`:93`) is rewritten as an `INSERT ... SELECT` with the visibility
check inside the statement — otherwise a user who guesses an id subscribes to
someone else's personal feed:

```sql
INSERT INTO radar_feed_subscriptions (user_id, feed_id)
SELECT $1, f.id FROM radar_feeds f
WHERE f.id = $2 AND (f.owner_user_id IS NULL OR f.owner_user_id = $1)
ON CONFLICT (user_id, feed_id)
  DO UPDATE SET created_at = radar_feed_subscriptions.created_at
RETURNING user_id, feed_id, created_at
```

Zero rows → `ErrFeedNotFound`. The `ON CONFLICT` idempotency is preserved.

`UpdateFeed` (`:616`) and `DeleteFeed` (`:677`) take an `owner *int64` parameter
and add one predicate to the `WHERE`:
`AND owner_user_id IS NOT DISTINCT FROM $n`. `IS NOT DISTINCT FROM` covers both
scopes in a single expression: `nil` matches global rows only, a value matches
that user's rows only. Missing the scope is indistinguishable from a missing
row; both yield `ErrNotFound`.

`SeedSubscriptions` (`:663`) — `WHERE is_active AND owner_user_id IS NULL`. A new
user has no personal feeds by definition.

`AddFeed` (`:75`) writes `owner_user_id` from `AddFeedParams`.

New methods: `GetGlobalFeedByURL(ctx, url string) (*Feed, error)` and
`CountUserFeeds(ctx, userID int64) (int, error)`.

`ListDueFeeds` (`:110`) and `MatchFindingToTopics` (`:237`) do not change. The
crawler works off `is_active` regardless of owner; matching already joins
`radar_feed_subscriptions`, and subscriptions are themselves bounded by
visibility now, so findings from a personal feed reach only its owner.

### Service (`internal/radar/service.go`)

Authorization moves from "trust the middleware" (today's "Admin scope;
middleware enforces" comments at `:332` and `:360`) to an explicit scope on
every method. Instead of one method with a flag, pairs:

```go
func (s *Service) AddUserFeed(ctx context.Context, userID int64, req AddFeedRequest) (*AddFeedResult, error)
func (s *Service) AddGlobalFeed(ctx context.Context, req AddFeedRequest) (*Feed, error)
func (s *Service) UpdateUserFeed(ctx context.Context, userID, feedID int64, req UpdateFeedRequest) (*Feed, error)
func (s *Service) UpdateGlobalFeed(ctx context.Context, feedID int64, req UpdateFeedRequest) (*Feed, error)
func (s *Service) DeleteUserFeed(ctx context.Context, userID, feedID int64) error
func (s *Service) DeleteGlobalFeed(ctx context.Context, feedID int64) error
```

Each pair delegates to a shared private helper taking `owner *int64`; the URL,
kind and interval validation from today's `AddFeed` (`:119-154`) moves into
`validateAddFeed` and is reused by both branches. `validateFetchInterval` is
already extracted.

`AddUserFeed` step by step: validate → `GetGlobalFeedByURL` (found → `Subscribe`,
return `AddFeedResult{Feed: existing, Created: false}`) → `CountUserFeeds`
against the quota → `AddFeed` with the owner → `Subscribe`. Two concurrent adds
at the quota boundary can land one feed over the limit; the check is
best-effort, recorded in a comment rather than in a lock.

The quota arrives as a variadic option so that no existing `NewService(store, emb)`
call site has to be rewritten (there are dozens across the tests):

```go
const defaultMaxUserFeeds = 20

// ServiceOption tunes optional Service behaviour.
type ServiceOption func(*Service)

// WithMaxUserFeeds caps how many personal feeds one account may own.
func WithMaxUserFeeds(n int) ServiceOption

func NewService(store StoreAPI, embedder embeddings.Client, opts ...ServiceOption) *Service
```

New types in `types.go`:

```go
// FeedScope narrows which catalog rows a read may see.
type FeedScope string

const (
	FeedScopeVisible FeedScope = "visible" // global feeds plus the caller's own
	FeedScopeGlobal  FeedScope = "global"  // the shared catalog only
)

// AddFeedResult reports whether a row was created or an existing catalog feed
// was reused (the caller was only subscribed to it).
type AddFeedResult struct {
	Feed    *Feed `json:"feed"`
	Created bool  `json:"created"`
}
```

`created` travels in the response body rather than only in the status code:
`apiFetch` (`web/src/shared/api/client.ts`) returns the parsed body and does not
surface the status, so reading the flag out of JSON is simpler than changing the
client. The status is still honest — 201 on creation, 200 on reuse.

`AddFeedParams` gains `OwnerUserID *int64`; `ListFeedsParams` gains
`Scope FeedScope` (the service normalizes an empty value to `FeedScopeVisible`).

Every new `StoreAPI` method (`:35`) must also appear on `mockStore`
(`internal/radar/service_test.go:18`): a compile-time
`var _ radar.StoreAPI = (*mockStore)(nil)` lives there, and without it the whole
test package fails to build.

### HTTP (`internal/radar/http.go`)

`AddFeedHandler` / `UpdateFeedHandler` / `DeleteFeedHandler` are renamed to
`AddGlobalFeedHandler` / `UpdateGlobalFeedHandler` / `DeleteGlobalFeedHandler`.
Added: `AddUserFeedHandler` (201 or 200 depending on `AddFeedResult.Created`),
`UpdateUserFeedHandler`, `DeleteUserFeedHandler`, `ListGlobalFeedsHandler`.

`listFeeds` (`:305`) sets `Scope: FeedScopeVisible`; the admin listing uses
`FeedScopeGlobal`. Both return the same `FeedList`: `is_own` is always `false`
on global rows, and `subscribed` is computed for the caller and simply ignored
on the admin screen — a separate DTO for one unused field is not worth it. The
stale "(admin)" notes on `ListFeedsParams` (`types.go:169`) and in the
`http.go:235` comment are removed at the same time: `GET /radar/feeds` has been
open to all roles since the previous phase.

### Routing (`internal/server/server.go`)

`POST/PATCH/DELETE /radar/feeds` move out of the `RequireAdmin` group
(`:163-168`) into the general user group and point at the `*UserFeed*` handlers.
A new block appears next to `r.Route("/radar", …)`:

```go
r.Route("/admin/radar", func(r chi.Router) {
	r.Use(coreauth.RequireUser(issuer))
	r.Use(coreauth.RequireAdmin)

	r.Get("/feeds", radarHTTP.ListGlobalFeedsHandler())
	r.Post("/feeds", radarHTTP.AddGlobalFeedHandler())
	r.Patch("/feeds/{id}", radarHTTP.UpdateGlobalFeedHandler())
	r.Delete("/feeds/{id}", radarHTTP.DeleteGlobalFeedHandler())
})
```

A separate `r.Route("/admin/radar", …)` rather than a nested `/admin` → `/radar`:
the future `/admin/users` lives outside the `cfg.RadarEnabled` branch and must
not disappear along with Radar. The else branch (`:171-173`) gains
`r.HandleFunc("/admin/radar", radar.DisabledHandler)` and `/admin/radar/*`.

`radar.NewService` receives `radar.WithMaxUserFeeds(cfg.RadarMaxUserFeeds)`.

### Config (`internal/core/config/config.go`)

```go
RadarMaxUserFeeds int `env:"LINKTHECA_RADAR_MAX_USER_FEEDS" envDefault:"20"`
```

next to the other `Radar*` fields (`:33-35`), plus a line in `.env.example`.

## Frontend

### Navigation

`Sidebar` (`web/src/shared/layout/Sidebar.tsx:5-9`) stops being static: the
component reads `useAuthStore((s) => s.user?.isAdmin ?? false)` and appends
`Admin → /admin/sources` as a fourth item. The `01/02/…` numbers are computed
from the index instead of hardcoded; Admin lands after Settings and the existing
numbers stay put.

A new `web/src/shared/layout/AdminRoute.tsx`, modelled on `ProtectedRoute.tsx`:
`bootstrapping` → `<FullPageSpinner/>` (otherwise a cold start produces a false
redirect), `anonymous` → `/login`, non-admin → `<Navigate to="/library" replace/>`.

In `App.tsx:24-57`, an `AdminRoute` branch is added inside
`ProtectedRoute > AppLayout` with `admin` → `<Navigate to="/admin/sources" replace/>`
and `admin/sources` → `AdminSourcesRoute`.

The gate is cosmetic; `RequireAdmin` on the backend is what tells the truth.

### Data

- `features/radar/types.ts:80` — `FeedListItem` gains `isOwn: boolean`.
- `features/radar/schemas.ts` — `RawFeedListItemSchema:172` gains `is_own`, and
  `mapFeedListItem:182` passes it through.
- `features/radar/api.ts:150-200` — `addFeed` returns `{ created: boolean }`,
  parsed out of the response body.
- A new `web/src/features/admin/` — `api.ts` (the `/admin/radar/feeds`
  endpoints, reusing `RawFeedListSchema` and `mapFeedListItem` from
  `features/radar/schemas`) and `use-admin-feeds.tsx` (`adminKeys.feeds`, a
  query and mutations modelled on `features/radar/use-mutations.tsx:138-167`).
  A separate feature rather than a branch inside radar: it is the scaffolding
  for the future `/admin/users`.

### Components

`SourceRow` (`features/radar/components/SourceRow.tsx`) — the `isAdmin` prop is
replaced by `canManage: boolean`, and `onToggle` becomes optional. Without
`onToggle` the checkbox is not rendered: on the admin screen subscription is
beside the point, the job there is curation.

`AddFeedDialog` — `mapFeedError` (`:38-45`) gains a `code === "quota_exceeded"`
branch → "You've reached the limit of N sources"; the 403 text stops saying
"only an instance admin can add feeds". The dialog is reused by both screens,
with the title and button label coming in as a prop.

`EditFeedDialog` is reused as is. `DeleteFeedConfirm` says "…will be removed for
all users" only for a global feed; for a personal one, "…will be removed".

### Screens

`web/src/routes/radar.sources.tsx` — two sections instead of a flat list, with
dividers copied from `web/src/routes/radar.topics._index.tsx:83-86`
(`label-sc-lg` + `flex-1 rule-dotted` + a count on the right):

```
Sources                                       [+ Add source]

MY SOURCES ·································· 1 / 20
☑  My Blog Digest                             [Edit] [Delete]
   example.com/rss · every 1h · fetched 4m ago · 12 items

CATALOG ····································· 8 feeds
☑  The Verge
   theverge.com/rss · every 1h · fetched 4m ago · 214 items
☐  Ars Technica
   arstechnica.com/feed · every 1h · ⚠ 404 Not Found (2d ago) · 61 items
```

Split by `feed.isOwn`, with `canManage={feed.isOwn}`. The "Add source" button is
now available to everyone and creates a personal feed. A 200 response (the URL
was already in the catalog) closes the dialog with the toast "Already in the
shared catalog — subscribed"; 201 gives "Source added".

Empty states: the personal section reads "Add your own RSS or Atom feed"; the
catalog reads "No shared sources yet. Ask the instance admin to add feeds."
`radar_disabled` still renders `RadarDisabled`.

A new `web/src/routes/admin.sources.tsx` — a `PageHeader` titled "Global
sources", subtitled "Feeds every account can subscribe to", with an "Add feed"
button; a list of `SourceRow` with `canManage` and no `onToggle`. Empty state:
"Add the first shared feed". Admin has no sub-navigation yet — with a single
item it would be noise; it arrives together with Users.

## Testing

Development is test-first.

**Go, store (real database):** user A cannot see user B's personal feed; two
users adding the same URL both succeed, the same user twice → `ErrDuplicate`, a
global feed twice → `ErrDuplicate`; `UpdateFeed`/`DeleteFeed` outside the scope →
`ErrNotFound`; `Subscribe` to a foreign personal feed → `ErrFeedNotFound`;
`SeedSubscriptions` ignores personal feeds; `CountUserFeeds` counts only one's
own; `ListFeeds` returns a `total` consistent with its scope.

**Go, service (mock store):** adding a URL from the global catalog creates no row
and returns `Created: false`; exceeding the quota → `ErrQuotaExceeded`;
`WithMaxUserFeeds` overrides the default; an empty patch still yields
`ErrInvalidInput`.

**Go, http (mock store):** `POST /radar/feeds` as an ordinary user → 201;
repeating the same URL → 409 `duplicate`; the quota → 409 `quota_exceeded`;
`GET /radar/feeds` returns `is_own`; `PATCH /radar/feeds/{id}` passes the userID
into `UpdateUserFeed`.

**Go, integration (real database plus a real router):**
`TestIntegrationAddFeedRequiresAdmin` (`integration_test.go:108`) is rewritten —
an ordinary user **may** now create a personal feed; a new test covers 403 on
`/admin/radar/feeds` for a non-admin; a finding from a personal feed reaches its
owner's topic and not another user's; deleting a user cascades away their feeds;
`/admin/radar/feeds` answers `radar_disabled` when Radar is off.

**Go, config:** `RadarMaxUserFeeds` defaults to 20 and can be overridden by env.

**Frontend (vitest + RTL + msw):** the Sidebar shows Admin only to an admin
(`Sidebar.test.tsx:20-25` hard-codes three links — update it); `AdminRoute`
redirects a non-admin and shows the spinner while `bootstrapping`;
`radar.sources` renders two sections, Edit/Delete only on personal rows, and
"Add source" visible to an ordinary user; a 200 response to an add produces the
existing-catalog toast; `admin.sources` renders Edit/Delete and no checkbox;
`AddFeedDialog` shows `quota_exceeded` inline.

## Documentation consequences

- `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` is marked
  superseded by this document. Only promote/demote and editing quotas from the
  UI remain deferred.
- In `docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md`, decision #1
  is marked as revised with a pointer here.
