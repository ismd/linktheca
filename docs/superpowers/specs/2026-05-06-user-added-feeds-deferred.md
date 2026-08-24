# User-added Radar feeds (deferred)

**Status:** deferred — to be designed alongside the Radar Settings UI.
**Decided:** 2026-05-06.

## Context

In phase 3a-2, `POST /radar/feeds` sits behind `RequireAdmin`. That is a
deliberate choice while the pipeline settles: feeds are an instance-level
resource, and open submission without quotas is a DoS vector against the crawler
and TEI.

## Target UX

The following vision is agreed:

- A **shared, curated feed catalog** exists, maintained by an admin
  (`POST /radar/feeds` behind `RequireAdmin` — today's behaviour).
- Every user has **their own set of subscriptions**, ticking any of the
  catalog's feeds on or off (`POST/DELETE /radar/subscriptions` — today's
  behaviour).
- Every user may additionally **add a personal feed of their own** to the
  catalog. Deduplication is by URL: if the URL is already there, the user simply
  gets a subscription to it. This part is new.

## What to do when we implement it

### 1. An endpoint for user submissions

Options:
- Drop `RequireAdmin` from `POST /radar/feeds` and branch on role (admin →
  curated, ordinary user → personal plus auto-subscribe).
- Or a separate `POST /radar/feeds/personal` (semantically cleaner).

On submission: if the URL already exists in `radar_feeds`, do not create a new
row, just record the subscription. If it does not, create the row and the
subscription in one transaction.

### 2. A `created_by` column

Add to `radar_feeds`:

```sql
ALTER TABLE radar_feeds
  ADD COLUMN created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
```

Semantics:
- `NULL` — curated (added by an admin or by a system seed). Every existing row at
  migration time is curated, so no backfill is needed.
- not NULL — added by a specific user (a personal feed).

Used in the UI to separate "the catalog" from "my feeds", and for quotas (below).

### 3. Quotas (admin-configurable)

Without quotas, an open endpoint is a DoS vector. At minimum:

- **A per-user limit on personal feeds**: the number of `radar_feeds` rows with
  `created_by = $user_id`. Default of 20, say.
- **A per-feed minimum interval**: a lower bound on `fetch_interval_seconds`, so
  a user cannot set 60s across a dozen feeds. Default of 1800s, say.
- **A global rate limit**: the total number of feeds on the instance.

All three are instance config an admin sets. Storage: either an
`instance_config` table (key/value) or env vars. To be settled at implementation
time.

### 4. Admin UI

- Promote: mark a personal feed as curated (`created_by → NULL`).
- Demote: the reverse.
- A list with a curated / personal filter.
- Editing the quotas.

### 5. Deletion

When a user "deletes their feed":
- If they are the only subscriber, delete the feed outright (the cascade takes
  the findings and matches with it).
- If there are other subscribers (after a promote-to-curated and then a demote,
  for instance), only unsubscribe the user and leave the feed in place.

## When to come back

Come back to this document while designing the Radar Settings UI (phase 3b or
later, presumably). Until then the current admin-only model is enough for every
smoke test and dogfood scenario.
