# Radar Sources UX — design

**Status:** approved, ready for implementation plan.
**Date:** 2026-08-15.

## Problem

Feeds (`radar_feeds`) can only be created by an admin through the API today, and
subscriptions (`radar_feed_subscriptions`) are made through a single endpoint,
`POST /radar/subscriptions`. There is nothing in the UI: a user cannot see the
catalog, cannot subscribe or unsubscribe, and an admin cannot fix or remove a
feed. Radar only works for whoever reaches it with curl.

## Scope

**In scope:** a feed catalog visible to every user; subscribing and
unsubscribing; admin add / edit / disable / delete; auto-subscribing a new user
to the whole active catalog; the crawler filling in feed titles.

**Out of scope:** users' personal feeds and the quotas that go with them
(`docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` still stands,
and the `created_by` column is not added); HTML scraping of non-RSS sources and
feed autodiscovery on a site; a manual "Fetch now"; per-topic sources (decided
earlier: subscriptions are per-user, not per-topic).

## Decisions

1. **One `/radar/sources` screen** inside the Radar section. Admin actions are
   embedded in the same rows; there is no separate admin area.
2. **Unsubscribing does not touch the past.** No new matches come from the feed,
   but the ones already found stay in the Inbox and in topics.
3. **A new user is subscribed to the whole active catalog** at registration.
   Feeds added later are not forced on anyone.
4. **One read endpoint, `GET /radar/feeds`**, for every role, with a
   `subscribed` field per row. Writes stay behind `RequireAdmin`.
5. **The crawler picks up the feed title**, and an admin's manual edit wins.

## Database schema

No migrations. `radar_feeds → radar_findings → radar_topic_matches` are already
linked by `ON DELETE CASCADE`, so deleting a feed cleans up its findings and
matches by itself.

## API

| Method | Path | Access | Behaviour |
|---|---|---|---|
| `GET` | `/radar/feeds` | user | The catalog. Pagination as today (`limit` ≤ 100, default 50). Sorting changes from `created_at DESC` to `lower(coalesce(title, url)) ASC`. |
| `POST` | `/radar/subscriptions` | user | Unchanged; idempotent through `ON CONFLICT`. |
| `DELETE` | `/radar/subscriptions/{feedId}` | user | 204. Unsubscribing from a feed you are not subscribed to is not an error. |
| `POST` | `/radar/feeds` | admin | Unchanged. A duplicate URL → 409 `duplicate`. |
| `PATCH` | `/radar/feeds/{id}` | admin | `title`, `fetch_interval_seconds`, `is_active` — all optional. An absent field means no change; `"title": ""` clears it (NULL in the database). |
| `DELETE` | `/radar/feeds/{id}` | admin | 204, cascade. |

A catalog row is a `FeedListItem`: the current `Feed` plus

- `subscribed bool` — whether the user in context has a subscription;
- `finding_count int` — how many findings the feed has produced; needed for the
  delete confirmation dialog.

`last_fetched_at` and `last_error` are visible to everyone: for a user they are
the only explanation of why a subscribed source has gone quiet. The instance is
self-hosted and its users are trusted.

### Errors

`ErrInvalidInput` → 400, a non-admin on a write → 403 (middleware),
`ErrNotFound` → 404, `ErrDuplicate` → 409, and `ErrFeedNotFound` → 404 when
subscribing to a deleted feed. The "an admin deleted the feed while a user was
clicking the checkbox" race resolves itself: the subscription answers 404, the
optimistic update rolls back, and the list is invalidated.

## Backend

### Matching

`Store.MatchFindingToTopics` already joins `radar_feed_subscriptions`, so
deleting a subscription row stops new matches on its own while leaving old ones
alone. The chosen unsubscribe semantics need no new logic.

### Auto-subscription at registration

`auth.ServiceConfig` gains a field

```go
OnUserCreated func(ctx context.Context, userID int64)
```

with no error return. `Service.Register` calls it after a successful
`CreateUser` if the field is non-nil. The error policy belongs to the caller: in
`server.go` the closure calls `radarSvc.SeedSubscriptions` and logs a failure
itself through `deps.Logger`. Registration does not fail — an unavailable Radar
must not block entry into the product. The hook is only installed inside the
`cfg.RadarEnabled` branch; with Radar off it is nil.

Seeding is one query:

```sql
INSERT INTO radar_feed_subscriptions (user_id, feed_id)
SELECT $1, id FROM radar_feeds WHERE is_active
ON CONFLICT DO NOTHING
```

There is no shared transaction with user creation. At worst a user starts with
no subscriptions and fixes it with checkboxes. A known consequence: anyone who
registers while Radar is off gets no auto-subscription — we catch that with an
empty state on the Sources screen, not with a migration.

### Feed title

Today nobody fills in `radar_feeds.title`: `Store.AddFeed` does not write it,
`crawler.Parse` returns only `feed.Items` and throws the channel title away, and
`MarkFeedFetched` never touches the column. So in a live database it is always
NULL — and match cards are already labelled with hosts (`MatchCard.tsx:22` does
`feedTitle ?? host(url)`).

Fixed as part of this feature:

- `crawler.Parse` returns the channel title along with the items (a
  `ParsedFeed{Title string; Items []*gofeed.Item}` struct instead of a bare
  slice);
- `CrawlFeed` (`internal/radar/jobs/crawl_feed.go`) passes it to
  `MarkFeedFetched`, which writes `title = COALESCE(radar_feeds.title, $n)` —
  that is, it fills in only what is empty.

An admin's manual title wins and is never overwritten by autofill. Clearing the
field in `EditFeedDialog` (`title → null`) brings the automatic name back on the
next sweep. The 304 Not Modified branch does not touch the title.

### New in `internal/radar`

- `Service.SeedSubscriptions(ctx, userID) error`
- `Service.Unsubscribe(ctx, userID, feedID) error`
- `Service.UpdateFeed(ctx, feedID, req) (*Feed, error)`
- `Service.DeleteFeed(ctx, feedID) error`
- `Service.ListFeeds` changes signature — it takes a `userID` for `subscribed`.
- Interval validation moves out of `AddFeed` into
  `validateFetchInterval(int) error` and is reused by `UpdateFeed`. An empty
  patch → `ErrInvalidInput` → 400, as with topics.
- `Store.UpdateFeed` — modelled on `Store.UpdateTopic`: a dynamic set of `SET`
  clauses, `RETURNING`, and 0 rows → `ErrNotFound`.
- `Store.ListFeeds` — `EXISTS (SELECT 1 FROM radar_feed_subscriptions …) AS
  subscribed` plus a `LEFT JOIN` with an aggregate over `radar_findings` for
  `finding_count` (in a single query, not N+1).
- `Store.Unsubscribe` and `Store.DeleteFeed` — plain `DELETE`s; the first stays
  quiet on 0 rows, the second returns `ErrNotFound`.

### Routing

`GET /radar/feeds` and `DELETE /radar/subscriptions/{feedId}` live in the shared
user group. `POST`, `PATCH`, and `DELETE` on `/radar/feeds` stay behind
`RequireAdmin`.

## Frontend

The `web/src/routes/radar.sources.tsx` route, registered in `App.tsx` as
`radar/sources`. The way in is a `Sources →` link in the `PageHeader` next to
the existing `Topics →` on the inbox and on the topics list. No `Sidebar` item is
added: Radar stays a single line of navigation.

```
Sources                                    [+ Add feed]   ← admin
Changes apply from the next sweep.

☑  The Verge                                  [edit] [✕]  ← admin
   theverge.com/rss · every 1h · fetched 4m ago · 214 items
──────────────────────────────────────────────────────────
☐  Ars Technica                               [edit] [✕]
   arstechnica.com/feed · every 1h · ⚠ 404 Not Found (2d ago) · 61 items
──────────────────────────────────────────────────────────
☑  Hacker News                                [edit] [✕]
   news.ycombinator.com/rss · paused · 1 203 items
```

`SourceRow`: a native subscription checkbox on the left, styled to the current
typography (no new primitive in `shared/ui`), the title and a meta line in the
middle, and the admin actions on the right. The title is `title`, falling back
to the hostname from the URL when it is null. The meta line is glued together
from the interval, the state of the last fetch (`fetched Nm ago` /
`⚠ <last_error> (Nd ago)` / `never fetched`), and `paused` when
`is_active=false`. A paused feed is dimmed but its checkbox still works —
subscribing ahead of time does no harm.

### Dialogs

Built on the existing `dialog.tsx` / `alert-dialog.tsx`, modelled on
`NewTopicDialog` and `DeleteTopicConfirm`.

- `AddFeedDialog` — a URL and an interval select (30m / 1h / 3h / 6h / 12h /
  24h). `kind` is not surfaced in the UI; the backend sets `rss`. A 409 is shown
  inline: "This feed is already in the catalog".
- `EditFeedDialog` — title (empty → `null`, and the automatic name returns on the
  next sweep), the interval, and a Paused toggle.
- `DeleteFeedConfirm` — "Delete *The Verge*? 214 findings and their matches will
  be removed for all users." The number comes from `finding_count`.

### State

The `radarKeys.feeds` key. Subscribe and unsubscribe are optimistic: flip
`subscribed` in the cached list and roll back on error (the pattern from
`use-mutations.tsx`). Admin mutations invalidate the list. Errors are toasts
through the already-wired `sonner`.

### Empty states

An empty catalog: an admin gets "Add the first feed" with a button, an ordinary
user gets "No sources yet. Ask the instance admin to add feeds." A
`radar_disabled` response renders `RadarDisabled`, as on the other radar routes.

Admin controls are hidden by `isAdmin` from the session. That is cosmetic;
`RequireAdmin` on the backend tells the truth.

## Testing

Development is test-first.

**Go, unit (`http_test.go`, mock store):** PATCH changes only the fields passed;
an empty patch → 400; an interval outside 300..86400 → 400; DELETE of a
nonexistent feed → 404; `GET /radar/feeds` returns `subscribed` for the user in
context; unsubscribe is idempotent (204 twice).

**Go, crawler and jobs:** `Parse` returns the channel title; `MarkFeedFetched`
fills an empty `title` and does not overwrite one already set; the 304 branch
leaves the title alone.

**Go, integration (`integration_test.go`, real database):** subscribe → a finding
matches → unsubscribe → a new finding does not match, and the old match is still
there; deleting a feed cascades away findings and matches; `SeedSubscriptions`
subscribes only to active feeds and is idempotent.

**Go, auth and server:** `Register` calls `OnUserCreated` with the created user's
id; with a nil hook registration works as before; with `RadarEnabled=false` the
hook is not installed.

**Frontend (vitest + RTL + msw):** the optimistic subscription toggle and its
rollback on a 500; admin controls do not render with `isAdmin: false`; the delete
confirmation shows `finding_count`; a 409 in `AddFeedDialog` is displayed
inline; an empty catalog gives different text to an admin and to a user.
