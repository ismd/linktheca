# Radar frontend (Topic UI) — design

**Date:** 2026-05-18
**Status:** approved, ready for writing-plans
**Scope:** the Radar UI on top of the finished read API, plus one targeted backend addition, `GET /radar/matches/{id}`. Full topic CRUD, reading matches, the match reader, mark-seen, and switching off the "Radar disabled" state.

## Context

The backend Radar read API is done (spec `2026-05-14-radar-read-api-design.md`).
The frontend foundation, auth, and Library are built; the sidebar carries a
`Radar (disabled)` item. The prototype in `prototype/index.html` sets the visual
language for Radar and the Topic view.

This phase turns that stubbed menu item into a live Radar section.

## Decisions taken during brainstorming

| # | Question | Decision |
|---|---|---|
| 1 | Scope | Full topic CRUD plus reading matches plus the match reader. Bulk "mark all seen" is deferred. |
| 2 | Keywords (the chips from the prototype) | Not shown. The embedding is computed from `name + description`; adding `topics.keywords` is deferred in [[project-radar-sources-not-per-topic]]. |
| 3 | Sources (the "Sources being watched" block from the prototype) | Not shown. Subscriptions are per-user (migration `008_radar_feed_subscriptions`), not per-topic. A concrete "sources for this topic" list is not defined in the model. `source_count` stays as a denormalized stat (a number, not a list). |
| 4 | Where a match click goes | An internal reader at `/radar/matches/$matchId`, visually like the library reader; the body is `summary`; a large "Open original" CTA; a "Save to library" action. |
| 5 | Match reader data fetching | Approach A: add a backend `GET /radar/matches/{id}` for direct-URL and refresh support. The alternatives (passing through router state, merging Topic view and reader) were rejected. |
| 6 | Mark-seen lifecycle | Automatic when the reader opens (a mount effect → PATCH /radar/matches/{id} state="seen"). |
| 7 | Embedder 503 on Create/Update | A specific toast, "Embedder unavailable, retry later". The generic toast is only for other errors. The modal/dialog does not close on a 503. |
| 8 | Threshold slider | Not shown in Create/Edit (`project-radar-threshold-slider-deferred`). The backend uses the default. |
| 9 | Subscription management | Not in the UI; an admin or the CLI adds feeds and the user subscribes through the API (`project-user-added-feeds-deferred`). |
| 10 | Full-content reader for a match | No; summary only (`project-radar-content-extraction-deferred`). |

## 1. Architecture

Mirrors the structure of `features/library/`, plus one backend extension.

### Frontend

```
web/src/features/radar/
  api.ts                  fetch functions, a thin wrapper over apiFetch
  schemas.ts              Zod schemas for the raw API + map functions snake_case → camelCase
  types.ts                TopicWithStats, MatchView, RadarStatus, FilterParams, PAGE_SIZE
  use-radar.tsx           useTopicsQuery, useTopicQuery, useMatchesQuery (infinite),
                          useMatchQuery, useRadarStatusQuery
  use-mutations.tsx       useCreateTopic, useUpdateTopic, useDeleteTopic, useMarkMatchSeen
  components/
    TopicCard.tsx
    TopicGrid.tsx
    MatchCard.tsx
    MatchGrid.tsx
    NewTopicDialog.tsx
    EditTopicDialog.tsx
    DeleteTopicConfirm.tsx
    TopicHeader.tsx
    StatsLine.tsx
    SkeletonCard.tsx      (if the library version cannot be reused as is)
    EmptyTopicList.tsx
    EmptyTopicMatches.tsx

web/src/routes/
  radar._index.tsx        /radar
  radar.$topicId.tsx      /radar/$topicId
  radar.matches.$matchId.tsx  /radar/matches/$matchId

web/src/shared/layout/Sidebar.tsx
  drop disabled: true from the Radar nav item
```

### Backend addition (`internal/radar/`)

A targeted extension, symmetrical with `GetTopic`.

| Layer | Method/Handler | Change |
|---|---|---|
| Store | `GetMatch(ctx, userID, matchID) (*MatchView, error)` | New. The SQL is the same JOIN as in `ListMatches`, plus `WHERE m.id = $1 AND t.user_id = $2`. Someone else's match → `ErrMatchNotFound`. |
| Service | `GetMatch(ctx, userID, matchID) (*MatchView, error)` | New, a pass-through. |
| HTTP | `GetMatchHandler() http.HandlerFunc` | New. 200/400/404/503. |
| Server | `r.Get("/matches/{id}", radarHTTP.GetMatchHandler())` | A new route inside the existing `r.Route("/radar", …)`. |
| StoreAPI / mockStore | extend the interface + the mock | Mandatory. |

No migrations, no new packages.

## 2. Routes and navigation

| Path | File | Purpose |
|---|---|---|
| `/radar` | `radar._index.tsx` | The Radar list: header + a grid of topic cards + last sweep + a "New topic" button. "On the radar" / "Paused" sections. |
| `/radar/$topicId` | `radar.$topicId.tsx` | Topic view: header (name/description) + StatsLine + matches on infinite scroll + Edit/Pause/Delete actions. |
| `/radar/matches/$matchId` | `radar.matches.$matchId.tsx` | Match reader: laid out like the library reader, body = summary, an "Open original" CTA, a "Save to library" action, and a back link to `/radar/$topicId`. |

All three sit under `_app.tsx` (sidebar + topbar) and `ProtectedRoute` (as
library does).

Sidebar: drop `disabled: true` from Radar in `Sidebar.tsx` (line 6).

## 3. Data layer

### API functions (`api.ts`)

```ts
listTopics(): Promise<TopicWithStats[]>
getTopic(id: number): Promise<TopicWithStats>
createTopic(input: { name: string; description: string; matchThreshold?: number }): Promise<TopicWithStats>
updateTopic(id: number, input: { name?: string; description?: string; isActive?: boolean; matchThreshold?: number }): Promise<TopicWithStats>
deleteTopic(id: number): Promise<void>
listMatches(args: { topicId?: number; state?: "new" | "seen"; limit: number; offset: number }): Promise<MatchList>
getMatch(id: number): Promise<MatchView>           // backend addition
updateMatch(id: number, input: { state: "new" | "seen" }): Promise<MatchView>
getStatus(): Promise<RadarStatus>
```

### Query keys

```ts
radarKeys = {
  all: ["radar"],
  topics: ["radar", "topics"],
  topic: (id) => ["radar", "topic", id],
  matches: (topicId, state) => ["radar", "matches", { topicId, state }],
  match: (id) => ["radar", "match", id],
  status: ["radar", "status"],
}
```

### Cache invalidation

| Mutation | Invalidate |
|---|---|
| `createTopic` | `topics` |
| `updateTopic` | `topics`, `topic(id)` |
| `deleteTopic` | `topics`; remove `topic(id)`, `matches(id, *)` |
| `markMatchSeen` | `matches(topicId, *)`, `topics` (newCount), `match(id)` |
| `useUpdateTopic` (pause/resume) | optimistic, rollback in `onError` |
| `useMarkMatchSeen` | optimistic, a single field |

### Schemas

The backend returns snake_case (`new_count`, `last_match_at`, `topic_id`,
`match_threshold`, `is_active`, `discovered_at`, `feed_title`). Zod schemas
decode the raw shape; map functions convert to camelCase. Date fields are parsed
into `Date | null`. `parseInDev` gates validation on DEV/test, as in library.

### Pagination

`useMatchesQuery` is an infinite query, `pageParam` = offset, `PAGE_SIZE` = 20.
`getNextPageParam` is identical to the library version.

## 4. Page components

### Radar list (`/radar`)

```
PageHeader        title="Radar"   subtitle=`Last sweep · ${fmt(status.lastSweepAt)}`
RadarToolbar      [+ New topic] (desktop right, mobile full-width below header)
<section>On the radar</section>
  TopicGrid (active=true)         grid-cols-1 md:grid-cols-2
<section>Paused</section>          (rendered only if there are paused topics)
  TopicGrid (active=false, opacity-60)
```

`TopicCard` renders a `TopicWithStats`:
- the index number at the top left, `stats.newCount` on the right (vermillion if
  > 0, a dash if 0)
- the name (display-tight 1.75rem)
- the description (line-clamp-2)
- a bottom line: `${stats.totalCount} found · ${stats.sourceCount} sources · ${fmt(stats.lastMatchAt)}`
- a Link to `/radar/$topicId`

**Not shown:** keyword chips, the sources list.

Loading: four `SkeletonCard`s. Empty: `EmptyTopicList` (a full-width "+ New
topic" CTA).

"Awaiting first sweep" replaces the timestamp when `status.lastSweepAt === null`.

### Topic view (`/radar/$topicId`)

```
BackLink          ← Back to radar
TopicHeader       name, description, Edit/Pause/Delete on the right (desktop)
StatsLine         <vermillion>{totalCount}</vermillion> found · {newCount} unread ·
                  {sourceCount} sources · created {fmt(createdAt)}
SectionHeader     "Found entries" · "{visibleCount} shown"
MatchGrid         infinite scroll, grid-cols-1 md:grid-cols-2 lg:grid-cols-3
```

`TopicHeader` actions:
- **Edit** → `EditTopicDialog` with the current values
- **Pause/Resume** → `useUpdateTopic({ isActive: !current })`, an optimistic
  toggle
- **Delete** → `DeleteTopicConfirm`, confirm → `useDeleteTopic` → navigate to
  `/radar`

`MatchCard`:
- top line: date · source · index
- title (display-tight, line-clamp-2)
- summary (line-clamp-3)
- a vermillion "new" stamp in the corner when `state === "new"`
- a Link to `/radar/matches/$matchId`

Empty: `EmptyTopicMatches` — "Standing watch. New entries will appear here."
A 404 on the topic → the not-found route.

### Match reader (`/radar/matches/$matchId`)

```
BackLink          ← Back to {match.topicName}     (navigate to /radar/${match.topicId})
ReaderHeader      topic stamp (a link to the topic), title, source · author · publishedAt · feedTitle
ReaderBody        summary (font-body, no drop cap)
                  fallback "No summary captured. Open original to read." when summary is empty
ReaderActions     [Open original ↗] (primary) · [Save to library] (secondary)
```

`useMatchQuery(matchId)` → a mount effect: if `match.state === "new"` →
`markSeen({ id, state: "seen" })`. Idempotent.

**"Save to library"** → `saveLink(match.finding.url)` (the existing
`library/api.ts`). A "Saved" toast plus invalidating `["library"]`. The match
stays in Radar as seen.

Layout components are reused from the library reader (`ReaderHeader`,
`ReaderActions`). If the interfaces do not line up, extract a shared core into
`shared/layout/Reader*` and wrap it from both sides.

Fallbacks:
- an empty `finding.title` → the URL instead of a heading
- an empty `finding.author` / `finding.feedTitle` → skip the line, no placeholder

### Dialogs

`NewTopicDialog` / `EditTopicDialog` — `shared/ui/dialog.tsx` (like
`AddLinkDialog`):
- Fields: `name` (required, max 200), `description` (required, textarea, max
  2000)
- Submit: `useCreateTopic` / `useUpdateTopic` → close plus a "Saved" toast
- A 503 from the embedder → an "Embedder unavailable, retry later" toast; the
  dialog does NOT close and the data is not lost
- A generic error → a "Could not save" toast; the dialog stays open

`DeleteTopicConfirm` — `shared/ui/alert-dialog.tsx`:
- Text: "Delete topic "{name}"? Matches will be lost." (the backend cascades away
  the matches; findings are untouched)
- Confirm → `useDeleteTopic` → navigate to `/radar`, a "Deleted" toast

## 5. Data flow

A typical path:
1. `/radar` mounts → `useTopicsQuery` and `useRadarStatusQuery` in parallel.
2. A card click → `/radar/$topicId` → `useTopicQuery(id)` plus
   `useMatchesQuery({topicId: id})`. The `topics` cache already holds this topic
   from step 1, so the header renders instantly through placeholder/initialData.
3. A match click → `/radar/matches/$matchId` → `useMatchQuery(id)`. If
   `match.state === "new"`, a mount effect fires `markSeenMutation`. An optimistic
   update on `matches(topicId, *)` and `topic.stats.newCount` (decrement) gives
   instant UI feedback in the sidebar and the list.
4. "Save to library" → `saveLink(finding.url)` → invalidate `["library"]`. The
   match stays seen.

### Mutations (the shared pattern)

- `mutationFn` → `onSuccess` (invalidate) plus `onError` (toast).
- `useUpdateTopic`/`useCreateTopic` distinguish a 503 (by HTTP status or
  `error.code === "embedder_unavailable"`) from everything else.
- Optimistic updates for `markMatchSeen` and Pause/Resume. Rollback in
  `onError`.

## 6. Edge cases and states

- **A direct URL to `/radar/matches/$matchId`** — `useMatchQuery` pulls the match
  by id; the BackLink uses `match.topicName` from the response.
- **A deleted topic with the reader open** — the next refetch returns 404 → the
  not-found UI.
- **An empty `last_sweep_at`** — "Awaiting first sweep" instead of a timestamp.
- **An empty `finding.summary`** — the fallback text in the reader body.
- **An empty `finding.title`** — the URL instead of a heading.
- **`LINKTHECA_RADAR_ENABLED=false`** — the backend returns `501 radar_disabled`
  on `/radar/*` (through `DisabledHandler`). The UI detects it by
  `error.code === "radar_disabled"` and renders a full-page "Radar is disabled in
  this instance." instead of the grid. The sidebar item stays visible (the module
  is present in the code).
- **Embedder unavailable** — the backend returns `503 embedder_unavailable`. In
  the Create/Edit dialog it becomes a specific toast; elsewhere (in the
  background, say) a generic toast.
- **401** — the existing logout flow (as in library).

## 7. Accessibility

- The dialog (Radix-based) — focus trap on, escape closes, ARIA labels.
- Cards — a wrapping `<Link>`, keyboard accessible (Enter/Space through React
  Router).
- Icon-only action buttons (Edit/Pause/Delete on mobile) — `aria-label`.
- Status announcements: the toast already uses `sonner` (a live region).

## 8. Testing

### Backend (Go, `internal/radar/`)

- `store_test.go` — `GetMatch` happy path / another user → 404 / nonexistent →
  404
- `service_test.go` — the mockStore pass-through plus errors
- `http_test.go` — 200 (JSON shape), 400 (bad id), 404, 503 disabled
- `integration_test.go` — add a `GET /radar/matches/{id}` step to the end-to-end
  scenario
- Update the `StoreAPI` interface and `mockStore`

### Frontend unit tests (`features/radar/`)

- `api.test.ts` — the fetch URLs, the query string for `listMatches`, the body
  shape for the `updateTopic` PATCH (only non-undefined fields)
- `schemas.test.ts` — Zod parsing of raw responses, snake_case → camelCase,
  null/optional fields
- `use-radar.test.tsx` — list, single, infinite pagination, the error path
- `use-mutations.test.tsx`:
  - `useCreateTopic` plus a 503 → the specific toast
  - `useUpdateTopic` optimistic plus rollback
  - `useMarkMatchSeen` invalidation
  - `useDeleteTopic` navigation

### Component tests

- `TopicCard.test.tsx` — rendering the stats, the vermillion newCount, paused
  dimming, the link href
- `NewTopicDialog.test.tsx` — submit, validation (empty name), 503 → the dialog
  stays open, the generic-error toast
- `EditTopicDialog.test.tsx` — initial values, a partial-update body (only the
  changed fields)
- Match reader **auto-mark-seen** — the mount effect fires the mutation exactly
  once when `state === "new"` and never when `state === "seen"` (a critical side
  effect)

### Not tested

- Trivial components (`StatsLine`, `EmptyTopicList`, `EmptyTopicMatches`) —
  covered indirectly.
- Layout components reused from library — already covered by the library tests.

### Manual smoke (before merge)

- Create a topic through the UI → it appears in the list → click → Topic view,
  empty → seed a match through the CLI → refresh → the match is visible → click →
  reader → newCount drops in the list → "Open original" opens a new tab → "Save
  to library" → check it in Library.
- Pause a topic → it moves into the Paused section → Resume → it comes back.
- Delete a topic → confirm → redirect to `/radar`, no matches.
- The blunt toggle: `LINKTHECA_RADAR_ENABLED=false` → the UI shows "Radar is
  disabled".

## 9. Out of scope (this iteration)

Recorded here so writing-plans does not reopen the discussion:

- Keyword chips in the Topic UI and a keywords field in the Create/Edit modal
- The sources block / a per-topic feed picker
- A subscription management UI
- The threshold slider in Create/Edit
- Bulk "Mark all seen" in the Topic view
- Full-content extraction for findings
- Cursor pagination
- A mobile-specific Radar layout beyond the media queries from the prototype
