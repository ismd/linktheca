# Radar inbox page

Design for [issue #9](https://github.com/ismd/linktheca/issues/9).

## Problem

`/radar` is a folder listing: to see a single result you must first pick a topic.
With six topics and four new articles spread across three of them, that is six
clicks to read four articles. The cost grows with the number of topics, so the
system penalises the behaviour it encourages ("create as many topics as you
want").

Library is an archive and browsing it by collection is right. Radar is a feed.
`/radar` should be an inbox of unread matches across all topics, sorted by time,
with topic management moved to a subpage.

## Scope

Frontend only. `GET /radar/matches` already accepts optional `topic_id` and
`state` and orders by `matched_at DESC` (`internal/radar/store.go:416`), and
`MatchView` already carries `topicName`. No API, schema, or Go changes.

## Routing

| URL | Contents | File |
| --- | --- | --- |
| `/radar` | Inbox: matches from all topics, `state=new` by default | `web/src/routes/radar._index.tsx` (rewritten) |
| `/radar/topics` | Topic grid, `+ New topic` | `web/src/routes/radar.topics._index.tsx` (moved from today's `radar._index.tsx`) |
| `/radar/topics/:topicId` | Topic archive | `web/src/routes/radar.topics.$topicId.tsx` (moved from `radar.$topicId.tsx`) |
| `/radar/matches/:matchId` | Reader | unchanged |

The `radar/:topicId` route is removed from `web/src/App.tsx`. No redirect from
the old URL: the app is self-hosted and nothing external links to those paths.

Link updates:

- `MatchReader` — back-link and topic stamp point to `/radar/topics/:id`.
- `TopicCard` — `to` points to `/radar/topics/:id`.
- `radar.topics.$topicId` — "← Back to radar" keeps pointing at `/radar`, which
  is now the inbox. Intentional: after reading a topic archive you land back in
  the feed.
- Sidebar is unchanged; `02 Radar` still points at `/radar`.

## Inbox page

### Filter state

Filters live in the URL via `useSearchParams`, mirroring
`web/src/routes/library._index.tsx`:

- `?state=all` — show every match. Absent or unrecognised means `new`.
- `?topic=<id>` — restrict to one topic. Absent or non-numeric means all topics.

Both defaults render no query parameters at all, so `/radar` is the canonical
inbox URL. Filter changes use `setParams(..., { replace: true })` as Library
does, so filter churn does not pile up in browser history.

The page's own filter type is `InboxFilters = { state: "new" | "all"; topicId?:
number }`. It is not `MatchState` (`"new" | "seen"`): `all` maps to
`MatchFilters.state === undefined`, which `buildMatchesQuery` omits from the
request, and `new` maps through unchanged.

`useMatchesQuery` already keys its cache on the filter object, so no changes to
`radarKeys`.

### Filter bar

New component `web/src/features/radar/components/InboxFilterBar.tsx`, styled
after `web/src/features/library/components/FilterBar.tsx`.

Props: `state: "new" | "all"`, `topicId: number | undefined`,
`topics: TopicWithStats[]`, `onChange: (next: InboxFilters) => void`.

Layout: a `New | All` toggle group, then a wrapping row of topic chips.

Chip rules:

- The row is `All topics` followed by one chip per topic drawn from: active
  topics, plus paused topics with `stats.newCount > 0`, plus the currently
  selected topic (so a filter arriving from the URL is always visible).
- Order follows `listTopics` (active first, then newest first). Chips are never
  sorted by count: re-sorting as you read makes them jump under the cursor.
- Each chip shows `stats.newCount`, hidden when zero.
- Clicking a chip selects that topic; clicking the already-selected chip resets
  to `All topics`.
- Active elements carry `aria-pressed`, as in `FilterBar`.

Counts come from `useTopicsQuery`, which the page already loads.

### Cards

`MatchCard` gains an optional `showTopic` boolean. When set, the topic name is
rendered first in the metadata row, before source and time:

```
[new] Rust · Hacker News · 2h                     01
```

The topic is plain text, not a link — the card itself is a `<Link>`, and nested
anchors are invalid HTML. Topic navigation stays available from the reader.

`MatchGrid` gains the same optional prop and passes it through. The inbox sets
it; the topic archive does not.

### Page composition

- `PageHeader title="Radar"`, subtitle `fmtSweep(status.lastSweepAt)` as today,
  and an `actions` slot holding a `Topics →` link to `/radar/topics`.
- `InboxFilterBar` under the header.
- `MatchGrid` with `showTopic`, plus the existing `Load more` button driven by
  `useMatchesQuery`'s `hasMore` / `fetchNextPage`.

Empty states, checked in this order:

1. No topics at all → `EmptyTopicList`, whose CTA opens the globally mounted
   `NewTopicDialog` via `useNewTopicStore` (`web/src/routes/__app.tsx:11`).
2. `state=new` and no matches → new `EmptyInbox` component ("Inbox zero", in the
   voice of the existing empty states).
3. `state=all` and no matches → reuse `EmptyTopicMatches`.

Radar-disabled handling is unchanged: if the topics query fails with
`ApiError.code === "radar_disabled"`, render the existing `RadarDisabled`
screen.

### Reading behaviour

Opening a match marks it `seen`, and `useMarkMatchSeen` already invalidates
`["radar", "matches"]` and `radarKeys.topics`. Returning to the inbox with the
default `new` filter therefore drops the card that was just read and decrements
its chip count. That is the intended inbox semantics; read matches remain
reachable through `All` and through the topic archive.

## Topics page

`/radar/topics` is today's `/radar` moved verbatim: `PageHeader`, the
`+ New topic` button, and the active/paused sections. The only edit is the
`PageHeader` subtitle, which becomes a short description of the page rather than
the sweep line (the sweep line belongs on the inbox). `TopicCard` links are
updated as noted above.

## Testing

TDD, colocated `*.test.tsx`, Vitest + React Testing Library, MSW for route-level
tests — following the existing files.

`MatchCard.test.tsx` (extend):

- with `showTopic`, the topic name renders in the metadata row;
- without it, the topic name is absent (regression guard for the archive view).

`InboxFilterBar.test.tsx` (new, no network):

- renders `All topics` plus a chip per active topic with its `newCount`;
- hides the count when `newCount` is zero;
- shows a paused topic with unread matches, hides a paused topic with none;
- shows the selected topic's chip even when the rules above would exclude it;
- clicking a chip emits that topic; clicking the active chip emits a reset;
- `aria-pressed` marks the active state and topic chip.

`radar._index.test.tsx` (new, MSW):

- with no search params, requests `/radar/matches` with `state=new` and no
  `topic_id`, and renders cards carrying the topic stamp;
- `?state=all` drops `state` from the request; `?topic=3` adds `topic_id=3`;
- the three empty states are chosen correctly;
- a `radar_disabled` error renders the "Radar is disabled" screen;
- the header links to `/radar/topics`.

Existing tests move with their files (`radar.topics._index`,
`radar.topics.$topicId`); only expected URLs change. `MatchReader.test.tsx` is
updated to expect the `/radar/topics/:id` back-link.

Verification: `npm test`, lint, and typecheck in `web/`. Go tests are untouched.

## Out of scope

- Redirect from the legacy `/radar/:topicId` URLs.
- Bulk actions in the inbox (mark-all-seen, per-card dismiss).
- Sorting controls; `matched_at DESC` remains the only order.
- Any backend change, including filtering paused topics server-side. Matches
  from paused topics stay in the inbox: pausing stops the search for new
  matches, it does not hide ones already found.
