# Radar Inbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `/radar` from a topic-folder listing into an inbox of unread matches across all topics, and move topic management to `/radar/topics`.

**Architecture:** Frontend only. `GET /radar/matches` already accepts optional `topic_id` and `state` and orders by `matched_at DESC`, and `MatchView` already carries `topicName`, so the whole feature is React routing + components. Filter state lives in the URL (`?state=all`, `?topic=3`) exactly as `web/src/routes/library._index.tsx` does it. Existing radar components (`MatchGrid`, `MatchCard`, `EmptyTopicMatches`, `EmptyTopicList`) are reused, gaining one optional prop where the inbox needs a different presentation.

**Tech Stack:** React 19 + TypeScript, react-router v7 (`createBrowserRouter` in `web/src/App.tsx`), TanStack Query v5, Tailwind, Vitest + React Testing Library + MSW.

Spec: `docs/superpowers/specs/2026-08-12-radar-inbox-design.md`. Issue: https://github.com/ismd/linktheca/issues/9

## Global Constraints

- All commands run from `web/` unless stated otherwise: `npm test`, `npm run lint`, `npm run typecheck`.
- No backend changes. Do not touch `internal/`, `migrations/`, or any Go file.
- Tests are colocated next to the code (`Foo.tsx` + `Foo.test.tsx`), Vitest + RTL, MSW handlers registered per-test with `server.use(...)`. `web/src/test/setup.ts` runs MSW with `onUnhandledRequest: "error"` — every request a test triggers must have a handler.
- MSW handler paths are prefixed with `/api` (e.g. `http.get("/api/radar/matches", ...)`) because `apiFetch` prepends `API_BASE = "/api"`.
- Tests that render anything hitting the API must seed auth first:
  ```ts
  beforeEach(() => {
    useAuthStore.getState().setSession("access", {
      id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
    });
  });
  ```
- Imports use the `@/` alias for `web/src` (e.g. `@/features/radar/types`), relative paths only within the same feature folder (e.g. `../types` from `features/radar/components/`).
- Tailwind classes come from the existing design vocabulary: `label-sc`, `display-tight`, `font-body`, `stamp`, `text-ink`, `text-ink-3`, `text-paper`, `text-muted-foreground`, `text-vermillion`, `bg-ink`, `bg-paper-2`, `border-rule`. Do not invent new utility names or add CSS.
- Commit after every task with a Conventional Commits message.

## File Structure

**Created:**
- `web/src/features/radar/components/InboxFilterBar.tsx` — the `New | All` toggle plus topic chips. Owns the chip-visibility rule.
- `web/src/features/radar/components/InboxFilterBar.test.tsx`
- `web/src/features/radar/components/EmptyInbox.tsx` — "Inbox zero" empty state.
- `web/src/features/radar/components/RadarDisabled.tsx` — extracted from today's `radar._index.tsx` so both the inbox and the topics page can render it.
- `web/src/features/radar/components/MatchGrid.test.tsx` — guards the `showTopic` pass-through.
- `web/src/routes/radar.topics._index.tsx` — today's topic grid page (moved).
- `web/src/routes/radar.topics.$topicId.tsx` — today's topic archive page (moved).
- `web/src/routes/radar._index.tsx` — rewritten from scratch as the inbox (after the move deletes the old one).
- `web/src/routes/radar._index.test.tsx`

**Modified:**
- `web/src/features/radar/types.ts` — add `InboxState` / `InboxFilters`.
- `web/src/features/radar/components/MatchCard.tsx` + `.test.tsx` — optional `showTopic`.
- `web/src/features/radar/components/MatchGrid.tsx` — optional `showTopic` pass-through.
- `web/src/features/radar/components/TopicCard.tsx` + `.test.tsx` — link to `/radar/topics/:id`.
- `web/src/features/radar/components/MatchReader.tsx` + `.test.tsx` — links to `/radar/topics/:id`.
- `web/src/App.tsx` — route table.

**Deleted:** `web/src/routes/radar.$topicId.tsx` (moved with `git mv`).

---

### Task 1: Topic stamp on match cards

**Files:**
- Modify: `web/src/features/radar/components/MatchCard.tsx`
- Modify: `web/src/features/radar/components/MatchGrid.tsx`
- Test: `web/src/features/radar/components/MatchCard.test.tsx` (extend)
- Test: `web/src/features/radar/components/MatchGrid.test.tsx` (create)

**Interfaces:**
- Consumes: `MatchView` from `web/src/features/radar/types.ts` (already has `topicName: string`).
- Produces: `MatchCard` and `MatchGrid` both accept an optional `showTopic?: boolean` prop, default `false`. Task 4 renders `<MatchGrid matches={...} showTopic />`.

- [ ] **Step 1: Write the failing tests**

Add to the existing `describe("MatchCard", ...)` block in `web/src/features/radar/components/MatchCard.test.tsx`. The fixture there already sets `topicName: "Local-first"`.

```tsx
  it("renders the topic name when showTopic is set", () => {
    r(<MatchCard match={match} index={0} showTopic />);
    expect(screen.getByText("Local-first")).toBeInTheDocument();
  });

  it("omits the topic name by default", () => {
    r(<MatchCard match={match} index={0} />);
    expect(screen.queryByText("Local-first")).toBeNull();
  });
```

Create `web/src/features/radar/components/MatchGrid.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { MatchGrid } from "./MatchGrid";
import type { MatchView } from "../types";

const match = (id: number, topicName: string): MatchView => ({
  id,
  topicId: id,
  topicName,
  similarity: 0.7,
  state: "new",
  matchedAt: new Date("2026-05-18T10:00:00Z"),
  finding: {
    id: id + 100,
    feedId: 5,
    feedTitle: "Ink & Switch",
    url: `https://example.com/${id}`,
    title: `Title ${id}`,
    summary: null,
    publishedAt: null,
    discoveredAt: new Date("2026-05-18T09:00:00Z"),
  },
});

describe("MatchGrid", () => {
  it("passes showTopic down to every card", () => {
    render(
      <MemoryRouter>
        <MatchGrid matches={[match(1, "Rust"), match(2, "Postgres")]} showTopic />
      </MemoryRouter>,
    );
    expect(screen.getByText("Rust")).toBeInTheDocument();
    expect(screen.getByText("Postgres")).toBeInTheDocument();
  });

  it("does not render topic names without showTopic", () => {
    render(
      <MemoryRouter>
        <MatchGrid matches={[match(1, "Rust")]} />
      </MemoryRouter>,
    );
    expect(screen.queryByText("Rust")).toBeNull();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm test -- src/features/radar/components/MatchCard.test.tsx src/features/radar/components/MatchGrid.test.tsx`

Expected: FAIL — `MatchCard.test.tsx` fails to typecheck/run on the unknown `showTopic` prop, `MatchGrid.test.tsx` fails on "Unable to find an element with the text: Rust".

- [ ] **Step 3: Add the prop to MatchCard**

In `web/src/features/radar/components/MatchCard.tsx`, replace the `Props` type and the metadata row:

```tsx
type Props = {
  match: MatchView;
  index: number;
  showTopic?: boolean;
};

export function MatchCard({ match, index, showTopic = false }: Props) {
  const f = match.finding;
  const title = f.title ?? host(f.url);
  const source = f.feedTitle ?? host(f.url);
  const stamp = match.state === "new";
  const when = f.publishedAt ?? f.discoveredAt;
  return (
    <Link to={`/radar/matches/${match.id}`} className="feed-card group block">
      <article className="flex flex-col h-full p-5 border border-rule">
        <div className="flex items-center gap-2 mb-3 flex-wrap">
          {stamp && <span className="stamp text-vermillion stamp-flat">new</span>}
          {showTopic && (
            <>
              <span className="label-sc text-ink">{match.topicName}</span>
              <span className="label-sc text-muted-foreground">·</span>
            </>
          )}
          <span className="label-sc text-muted-foreground">{source}</span>
```

Leave the rest of the file unchanged. The topic is plain text, not a `<Link>` — the card itself is already an anchor and nested anchors are invalid HTML.

- [ ] **Step 4: Add the pass-through to MatchGrid**

Replace the whole of `web/src/features/radar/components/MatchGrid.tsx`:

```tsx
import type { MatchView } from "../types";
import { MatchCard } from "./MatchCard";

type Props = {
  matches: MatchView[];
  showTopic?: boolean;
};

export function MatchGrid({ matches, showTopic }: Props) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {matches.map((m, i) => (
        <MatchCard key={m.id} match={m} index={i} showTopic={showTopic} />
      ))}
    </div>
  );
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `npm test -- src/features/radar/components/MatchCard.test.tsx src/features/radar/components/MatchGrid.test.tsx`

Expected: PASS, all tests in both files.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/radar/components/MatchCard.tsx web/src/features/radar/components/MatchCard.test.tsx web/src/features/radar/components/MatchGrid.tsx web/src/features/radar/components/MatchGrid.test.tsx
git commit -m "feat(radar): show topic name on match cards via showTopic prop"
```

---

### Task 2: Inbox filter bar

**Files:**
- Modify: `web/src/features/radar/types.ts`
- Create: `web/src/features/radar/components/InboxFilterBar.tsx`
- Test: `web/src/features/radar/components/InboxFilterBar.test.tsx`

**Interfaces:**
- Consumes: `TopicWithStats` from `web/src/features/radar/types.ts` (has `id`, `name`, `isActive`, `stats.newCount`).
- Produces:
  - `type InboxState = "new" | "all"` and `type InboxFilters = { state: InboxState; topicId?: number }` exported from `web/src/features/radar/types.ts`.
  - `InboxFilterBar` with props `{ state: InboxState; topicId: number | undefined; topics: TopicWithStats[]; onChange: (next: InboxFilters) => void }`, default-exported as a named export `InboxFilterBar`.
  - Task 4 imports both.

Note: `InboxState` is deliberately **not** `MatchState` (`"new" | "seen"`). Task 4 maps `all` → `state: undefined` when calling `useMatchesQuery`.

- [ ] **Step 1: Add the filter types**

In `web/src/features/radar/types.ts`, append below the existing `MatchFilters` type:

```ts
export type InboxState = "new" | "all";

export type InboxFilters = {
  state: InboxState;
  topicId?: number;
};
```

- [ ] **Step 2: Write the failing test**

Create `web/src/features/radar/components/InboxFilterBar.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InboxFilterBar } from "./InboxFilterBar";
import type { TopicWithStats } from "../types";

function topic(
  id: number,
  name: string,
  opts: { isActive?: boolean; newCount?: number } = {},
): TopicWithStats {
  return {
    id,
    userId: 1,
    name,
    description: "D",
    matchThreshold: 0.55,
    isActive: opts.isActive ?? true,
    hasEmbedding: true,
    createdAt: new Date("2026-05-01T10:00:00Z"),
    updatedAt: new Date("2026-05-02T10:00:00Z"),
    stats: {
      newCount: opts.newCount ?? 0,
      totalCount: 10,
      sourceCount: 2,
      lastMatchAt: null,
    },
  };
}

describe("InboxFilterBar", () => {
  it("renders All topics plus a chip per active topic with its new count", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 4 }), topic(2, "Postgres", { newCount: 2 })]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "All topics" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Rust 4" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Postgres 2" })).toBeInTheDocument();
  });

  it("hides the count when a topic has no new matches", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 0 })]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Rust" })).toBeInTheDocument();
  });

  it("shows a paused topic with unread matches and hides a paused topic without", () => {
    render(
      <InboxFilterBar
        state="new"
        topicId={undefined}
        topics={[
          topic(1, "Paused loud", { isActive: false, newCount: 3 }),
          topic(2, "Paused quiet", { isActive: false, newCount: 0 }),
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Paused loud 3" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Paused quiet" })).toBeNull();
  });

  it("shows the selected topic even when it would otherwise be hidden", () => {
    render(
      <InboxFilterBar
        state="all"
        topicId={2}
        topics={[topic(2, "Paused quiet", { isActive: false, newCount: 0 })]}
        onChange={vi.fn()}
      />,
    );
    const chip = screen.getByRole("button", { name: "Paused quiet" });
    expect(chip).toHaveAttribute("aria-pressed", "true");
  });

  it("emits the topic id on click and keeps the current state", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="all"
        topicId={undefined}
        topics={[topic(1, "Rust", { newCount: 4 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Rust 4" }));
    expect(onChange).toHaveBeenCalledWith({ state: "all", topicId: 1 });
  });

  it("clears the topic when the active chip is clicked again", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="new"
        topicId={1}
        topics={[topic(1, "Rust", { newCount: 4 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Rust 4" }));
    expect(onChange).toHaveBeenCalledWith({ state: "new", topicId: undefined });
  });

  it("emits the new state while keeping the selected topic", async () => {
    const onChange = vi.fn();
    render(
      <InboxFilterBar
        state="new"
        topicId={3}
        topics={[topic(3, "Rust", { newCount: 1 })]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(onChange).toHaveBeenCalledWith({ state: "all", topicId: 3 });
  });

  it("marks the active state button with aria-pressed", () => {
    render(
      <InboxFilterBar state="new" topicId={undefined} topics={[]} onChange={vi.fn()} />,
    );
    expect(screen.getByRole("button", { name: "New" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "All" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `npm test -- src/features/radar/components/InboxFilterBar.test.tsx`

Expected: FAIL with "Failed to resolve import ./InboxFilterBar".

- [ ] **Step 4: Write the component**

Create `web/src/features/radar/components/InboxFilterBar.tsx`:

```tsx
import type { InboxFilters, InboxState, TopicWithStats } from "../types";

type Props = {
  state: InboxState;
  topicId: number | undefined;
  topics: TopicWithStats[];
  onChange: (next: InboxFilters) => void;
};

const STATES: { label: string; value: InboxState }[] = [
  { label: "New", value: "new" },
  { label: "All", value: "all" },
];

const CHIP_ACTIVE = "px-3 py-1.5 label-sc bg-ink text-paper inline-flex items-center gap-2";
const CHIP_IDLE = "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2 inline-flex items-center gap-2";

// Active topics are always offered. A paused topic only earns a chip while it
// still has unread matches — or while it is the current filter, so a selection
// arriving from the URL is never invisible.
export function visibleTopics(
  topics: TopicWithStats[],
  selectedId: number | undefined,
): TopicWithStats[] {
  return topics.filter(
    (t) => t.isActive || t.stats.newCount > 0 || t.id === selectedId,
  );
}

export function InboxFilterBar({ state, topicId, topics, onChange }: Props) {
  const chips = visibleTopics(topics, topicId);
  return (
    <div className="py-4 border-b border-rule">
      <div className="flex flex-wrap gap-1" role="group" aria-label="State filter">
        {STATES.map((opt) => {
          const active = state === opt.value;
          return (
            <button
              key={opt.value}
              type="button"
              aria-pressed={active}
              onClick={() => onChange({ state: opt.value, topicId })}
              className={
                active
                  ? "px-3 py-1.5 label-sc bg-ink text-paper cursor-auto"
                  : "px-3 py-1.5 label-sc text-ink-3 hover:bg-paper-2"
              }
            >
              {opt.label}
            </button>
          );
        })}
      </div>

      <div className="flex flex-wrap gap-1 mt-3" role="group" aria-label="Topic filter">
        <button
          type="button"
          aria-pressed={topicId === undefined}
          onClick={() => onChange({ state, topicId: undefined })}
          className={topicId === undefined ? CHIP_ACTIVE : CHIP_IDLE}
        >
          All topics
        </button>
        {chips.map((t) => {
          const active = topicId === t.id;
          return (
            <button
              key={t.id}
              type="button"
              aria-pressed={active}
              onClick={() => onChange({ state, topicId: active ? undefined : t.id })}
              className={active ? CHIP_ACTIVE : CHIP_IDLE}
            >
              {t.name}
              {t.stats.newCount > 0 && (
                <span className={active ? "text-paper" : "text-vermillion"}>
                  {t.stats.newCount}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
```

Note on the tests' accessible names: RTL joins a button's text nodes with a space, so `Rust` + `4` matches the name `"Rust 4"`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `npm test -- src/features/radar/components/InboxFilterBar.test.tsx`

Expected: PASS, 8 tests.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/radar/types.ts web/src/features/radar/components/InboxFilterBar.tsx web/src/features/radar/components/InboxFilterBar.test.tsx
git commit -m "feat(radar): add InboxFilterBar with state toggle and topic chips"
```

---

### Task 3: Move topic pages under /radar/topics

Purely a relocation: no new UI. At the end of this task `/radar` and `/radar/topics` both render the topic grid — `/radar` gets replaced by the inbox in Task 4.

**Files:**
- Move: `web/src/routes/radar._index.tsx` → `web/src/routes/radar.topics._index.tsx`
- Move: `web/src/routes/radar.$topicId.tsx` → `web/src/routes/radar.topics.$topicId.tsx`
- Create: `web/src/features/radar/components/RadarDisabled.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/features/radar/components/TopicCard.tsx`
- Modify: `web/src/features/radar/components/MatchReader.tsx`
- Test: `web/src/features/radar/components/TopicCard.test.tsx` (update), `web/src/features/radar/components/MatchReader.test.tsx` (extend)

**Interfaces:**
- Produces:
  - `RadarDisabled` — a zero-prop named export from `web/src/features/radar/components/RadarDisabled.tsx`. Task 4 imports it.
  - Routes `/radar/topics` and `/radar/topics/:topicId`.
  - `web/src/routes/radar._index.tsx` no longer exists; Task 4 creates it fresh.

- [ ] **Step 1: Update the link assertions (failing tests first)**

In `web/src/features/radar/components/TopicCard.test.tsx` line 27, change the expected href:

```tsx
    expect(link).toHaveAttribute("href", "/radar/topics/7");
```

In `web/src/features/radar/components/MatchReader.test.tsx`, add a test inside the existing `describe("MatchReader", ...)` block. That file's `rawMatch(state)` fixture serves match id 42 with `topic_id: 7`, and its harness is the `<Wrap>` component defined at the top of the file — reuse both, do not introduce a second harness.

```tsx
  it("links back to the topic archive under /radar/topics", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("seen"))),
    );
    render(
      <Wrap>
        <MatchReader matchId={42} />
      </Wrap>,
    );
    const links = await screen.findAllByRole("link");
    const topicLinks = links.filter(
      (l) => l.getAttribute("href") === "/radar/topics/7",
    );
    expect(topicLinks).toHaveLength(2);
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm test -- src/features/radar/components/TopicCard.test.tsx src/features/radar/components/MatchReader.test.tsx`

Expected: FAIL — TopicCard expects `/radar/topics/7` but gets `/radar/7`; MatchReader finds 0 links with `/radar/topics/1`.

- [ ] **Step 3: Update the links**

`web/src/features/radar/components/TopicCard.tsx`:

```tsx
      to={`/radar/topics/${topic.id}`}
```

`web/src/features/radar/components/MatchReader.tsx` — both occurrences (the back-link and the topic stamp in the header):

```tsx
        to={`/radar/topics/${m.topicId}`}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm test -- src/features/radar/components/TopicCard.test.tsx src/features/radar/components/MatchReader.test.tsx`

Expected: PASS.

- [ ] **Step 5: Move the route files**

```bash
git mv web/src/routes/radar._index.tsx web/src/routes/radar.topics._index.tsx
git mv web/src/routes/radar.\$topicId.tsx web/src/routes/radar.topics.\$topicId.tsx
```

- [ ] **Step 6: Extract RadarDisabled**

Create `web/src/features/radar/components/RadarDisabled.tsx` with the component lifted verbatim out of the moved `radar.topics._index.tsx`:

```tsx
export function RadarDisabled() {
  return (
    <div className="text-center py-20">
      <p className="display-tight text-3xl text-ink mb-3">Radar is disabled</p>
      <p className="font-body italic text-muted-foreground">
        This Linktheca instance was started with Radar turned off.
      </p>
    </div>
  );
}
```

In `web/src/routes/radar.topics._index.tsx`: delete the local `RadarDisabled` function and import it instead:

```tsx
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
```

Also rename the default export to match its new job and swap the subtitle — the sweep line belongs on the inbox from now on:

```tsx
export default function TopicsListRoute() {
```

```tsx
      <PageHeader title="Topics" subtitle="Everything on your radar" />
```

Then delete the now-unused `useRadarStatusQuery` and `fmtSweep` imports and the `const status = useRadarStatusQuery();` line from that file. Add a back-link to the inbox directly above the `PageHeader`'s sibling content — as the first child inside the `px-4 lg:px-8 pb-6 pt-6` wrapper:

```tsx
        <Link
          to="/radar"
          className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-6"
        >
          ← Back to inbox
        </Link>
```

with `import { Link } from "react-router";` at the top of the file.

`web/src/routes/radar.topics.$topicId.tsx` needs no edits: its default export stays `TopicRoute`, and its "← Back to radar" link keeps pointing at `/radar`, which becomes the inbox — that is intentional.

- [ ] **Step 7: Update the route table**

In `web/src/App.tsx`, replace the two radar imports and the radar route entries:

```tsx
import TopicsListRoute from "./routes/radar.topics._index";
import TopicRoute from "./routes/radar.topics.$topicId";
import MatchRoute from "./routes/radar.matches.$matchId";
```

```tsx
              { path: "radar", element: <TopicsListRoute /> },
              { path: "radar/topics", element: <TopicsListRoute /> },
              { path: "radar/topics/:topicId", element: <TopicRoute /> },
              { path: "radar/matches/:matchId", element: <MatchRoute /> },
```

(The `radar` entry is temporary scaffolding so the app stays usable; Task 4 points it at the inbox.)

- [ ] **Step 8: Verify the whole suite and the typechecker**

Run: `npm test && npm run typecheck && npm run lint`

Expected: all tests pass, no type errors, no lint warnings. If `typecheck` reports an unused import in `radar.topics._index.tsx`, remove it.

- [ ] **Step 9: Commit**

```bash
git add web/src
git commit -m "refactor(radar): move topic pages under /radar/topics"
```

---

### Task 4: The inbox page

**Files:**
- Create: `web/src/routes/radar._index.tsx`
- Create: `web/src/routes/radar._index.test.tsx`
- Create: `web/src/features/radar/components/EmptyInbox.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: `InboxFilterBar` + `InboxFilters` / `InboxState` (Task 2), `MatchGrid`'s `showTopic` (Task 1), `RadarDisabled` (Task 3), plus the existing `useTopicsQuery`, `useMatchesQuery`, `useRadarStatusQuery` from `@/features/radar/use-radar`, `useNewTopicStore` from `@/features/radar/use-new-topic-store`, `fmtSweep` from `@/features/radar/time`, and `EmptyTopicList` / `EmptyTopicMatches` from `@/features/radar/components/`.
- Produces: default export `RadarInboxRoute` at `/radar`.

- [ ] **Step 1: Write the failing route test**

Create `web/src/routes/radar._index.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import RadarInboxRoute from "./radar._index";

const rawTopic = (id: number, name: string, newCount: number) => ({
  id,
  user_id: 1,
  name,
  description: "D",
  match_threshold: 0.55,
  is_active: true,
  has_embedding: true,
  created_at: "2026-05-01T10:00:00Z",
  updated_at: "2026-05-02T10:00:00Z",
  stats: {
    new_count: newCount,
    total_count: 10,
    source_count: 2,
    last_match_at: null,
  },
});

const rawMatch = (id: number, topicName: string) => ({
  id,
  topic_id: 1,
  topic_name: topicName,
  similarity: 0.7,
  state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: id + 100,
    feed_id: 5,
    feed_title: "Ink & Switch",
    url: `https://example.com/${id}`,
    title: `Title ${id}`,
    summary: null,
    published_at: null,
    discovered_at: "2026-05-18T09:00:00Z",
  },
});

type Scenario = {
  topics?: unknown[];
  matches?: unknown[];
  topicsError?: { status: number; body: unknown };
};

const seen: string[] = [];

function stub(s: Scenario) {
  server.use(
    http.get("/api/radar/status", () =>
      HttpResponse.json({ last_sweep_at: "2026-05-18T11:00:00Z" }),
    ),
    http.get("/api/radar/topics", () =>
      s.topicsError
        ? HttpResponse.json(s.topicsError.body, { status: s.topicsError.status })
        : HttpResponse.json({ items: s.topics ?? [] }),
    ),
    http.get("/api/radar/matches", ({ request }) => {
      seen.push(request.url);
      return HttpResponse.json({
        items: s.matches ?? [],
        total: (s.matches ?? []).length,
      });
    }),
  );
}

function renderAt(path: string) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={qc}>
        <Routes>
          <Route path="/radar" element={<RadarInboxRoute />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  seen.length = 0;
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("RadarInboxRoute", () => {
  it("requests unread matches across all topics by default", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    const url = new URL(seen[0]);
    expect(url.searchParams.get("state")).toBe("new");
    expect(url.searchParams.get("topic_id")).toBeNull();
  });

  it("stamps each card with its topic name", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    // The chip carries the name too, so assert on more than one match.
    expect(screen.getAllByText("Rust").length).toBeGreaterThan(1);
  });

  it("drops the state parameter when ?state=all", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?state=all");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(new URL(seen[0]).searchParams.get("state")).toBeNull();
  });

  it("sends topic_id when ?topic is set", async () => {
    stub({ topics: [rawTopic(3, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?topic=3");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(new URL(seen[0]).searchParams.get("topic_id")).toBe("3");
  });

  it("ignores a non-numeric topic parameter", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar?topic=abc");

    expect(await screen.findByText("Title 1")).toBeInTheDocument();
    expect(new URL(seen[0]).searchParams.get("topic_id")).toBeNull();
  });

  it("prompts to create a topic when there are none", async () => {
    stub({ topics: [], matches: [] });
    renderAt("/radar");

    expect(await screen.findByText("Nothing on your radar yet")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "All topics" })).toBeNull();
  });

  it("shows inbox zero when nothing is unread", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [] });
    renderAt("/radar");

    expect(await screen.findByText("Inbox zero")).toBeInTheDocument();
  });

  it("shows the standing-watch empty state when All is empty", async () => {
    stub({ topics: [rawTopic(1, "Rust", 0)], matches: [] });
    renderAt("/radar?state=all");

    expect(await screen.findByText("Nothing yet")).toBeInTheDocument();
    expect(screen.queryByText("Inbox zero")).toBeNull();
  });

  it("renders the disabled screen when radar is off", async () => {
    stub({
      topicsError: {
        status: 403,
        body: {
          error: "radar_disabled",
          message: "radar feature is disabled on this server",
        },
      },
    });
    renderAt("/radar");

    expect(await screen.findByText("Radar is disabled")).toBeInTheDocument();
  });

  it("links to the topics page from the header", async () => {
    stub({ topics: [rawTopic(1, "Rust", 1)], matches: [rawMatch(1, "Rust")] });
    renderAt("/radar");

    expect(await screen.findByRole("link", { name: /topics/i })).toHaveAttribute(
      "href",
      "/radar/topics",
    );
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm test -- src/routes/radar._index.test.tsx`

Expected: FAIL with "Failed to resolve import ./radar._index".

- [ ] **Step 3: Write the EmptyInbox component**

Create `web/src/features/radar/components/EmptyInbox.tsx`:

```tsx
export function EmptyInbox() {
  return (
    <div className="text-center py-16 border border-dashed border-rule">
      <p className="label-sc text-muted-foreground mb-3">Inbox zero</p>
      <p className="font-body italic text-muted-foreground">
        Nothing new since you last looked. Standing watch.
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Write the inbox route**

Create `web/src/routes/radar._index.tsx`:

```tsx
import { Link, useSearchParams } from "react-router";
import { ApiError } from "@/shared/api/errors";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import {
  useTopicsQuery,
  useMatchesQuery,
  useRadarStatusQuery,
} from "@/features/radar/use-radar";
import { useNewTopicStore } from "@/features/radar/use-new-topic-store";
import { fmtSweep } from "@/features/radar/time";
import { InboxFilterBar } from "@/features/radar/components/InboxFilterBar";
import { MatchGrid } from "@/features/radar/components/MatchGrid";
import { EmptyInbox } from "@/features/radar/components/EmptyInbox";
import { EmptyTopicList } from "@/features/radar/components/EmptyTopicList";
import { EmptyTopicMatches } from "@/features/radar/components/EmptyTopicMatches";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
import type { InboxFilters } from "@/features/radar/types";

function parseFilters(params: URLSearchParams): InboxFilters {
  const topic = Number(params.get("topic"));
  return {
    state: params.get("state") === "all" ? "all" : "new",
    topicId: Number.isInteger(topic) && topic > 0 ? topic : undefined,
  };
}

function nextSearch(
  current: URLSearchParams,
  next: InboxFilters,
): URLSearchParams {
  const out = new URLSearchParams(current);
  if (next.state === "all") out.set("state", "all");
  else out.delete("state");
  if (next.topicId !== undefined) out.set("topic", String(next.topicId));
  else out.delete("topic");
  return out;
}

export default function RadarInboxRoute() {
  const [params, setParams] = useSearchParams();
  const filters = parseFilters(params);
  const topics = useTopicsQuery();
  const status = useRadarStatusQuery();
  const matches = useMatchesQuery({
    topicId: filters.topicId,
    state: filters.state === "new" ? "new" : undefined,
  });
  const openNewTopic = useNewTopicStore((s) => s.open);

  if (
    topics.error instanceof ApiError &&
    topics.error.code === "radar_disabled"
  ) {
    return <RadarDisabled />;
  }

  const noTopics = topics.isSuccess && topics.data.length === 0;

  return (
    <div>
      <PageHeader
        title="Radar"
        subtitle={fmtSweep(status.data?.lastSweepAt ?? null)}
        actions={
          <Link
            to="/radar/topics"
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Topics →
          </Link>
        }
      />
      <div className="px-4 lg:px-8 pb-10">
        {noTopics ? (
          <div className="pt-8">
            <EmptyTopicList onAdd={openNewTopic} />
          </div>
        ) : (
          <>
            <InboxFilterBar
              state={filters.state}
              topicId={filters.topicId}
              topics={topics.data ?? []}
              onChange={(next) =>
                setParams(nextSearch(params, next), { replace: true })
              }
            />
            <div className="pt-8">
              {matches.isLoading ? (
                <p className="font-body italic text-muted-foreground">Loading…</p>
              ) : matches.items.length === 0 ? (
                filters.state === "new" ? (
                  <EmptyInbox />
                ) : (
                  <EmptyTopicMatches />
                )
              ) : (
                <>
                  <MatchGrid matches={matches.items} showTopic />
                  {matches.hasMore && (
                    <div className="flex justify-center mt-10">
                      <Button
                        variant="outline"
                        onClick={() => matches.fetchNextPage()}
                        disabled={matches.isFetchingNextPage}
                      >
                        {matches.isFetchingNextPage ? "Loading…" : "Load more"}
                      </Button>
                    </div>
                  )}
                </>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Point /radar at the inbox**

In `web/src/App.tsx`, add the import and replace the temporary `radar` entry from Task 3:

```tsx
import RadarInboxRoute from "./routes/radar._index";
```

```tsx
              { path: "radar", element: <RadarInboxRoute /> },
              { path: "radar/topics", element: <TopicsListRoute /> },
              { path: "radar/topics/:topicId", element: <TopicRoute /> },
              { path: "radar/matches/:matchId", element: <MatchRoute /> },
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `npm test -- src/routes/radar._index.test.tsx`

Expected: PASS, 10 tests. (The disabled-radar stub mirrors the real backend: `internal/radar/http.go:48` replies `403` with the `{ error, message }` envelope from `internal/core/httpx`, and `apiFetch` reads `error` into `ApiError.code`.)

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/radar._index.tsx web/src/routes/radar._index.test.tsx web/src/features/radar/components/EmptyInbox.tsx web/src/App.tsx
git commit -m "feat(radar): make /radar an inbox of unread matches across topics"
```

---

### Task 5: Full verification

**Files:** none created; fixes only if something below fails.

**Interfaces:** none.

- [ ] **Step 1: Run the full frontend suite**

Run (from `web/`): `npm test`

Expected: every suite passes. Radar suites in play: `api.test.ts`, `schemas.test.ts`, `use-radar.test.tsx`, `use-mutations.test.tsx`, `MatchCard.test.tsx`, `MatchGrid.test.tsx`, `MatchReader.test.tsx`, `TopicCard.test.tsx`, `InboxFilterBar.test.tsx`, `radar._index.test.tsx`, plus `Sidebar.test.tsx` (still expects `/radar`, which is correct — the sidebar points at the inbox).

- [ ] **Step 2: Typecheck, lint, build**

Run: `npm run typecheck && npm run lint && npm run build`

Expected: clean. `npm run build` catches route-file imports that Vitest tolerated.

- [ ] **Step 3: Grep for stale topic URLs**

Run: `grep -rn 'to={\`/radar/\${' web/src; grep -rn '"/radar/[0-9]' web/src`

Expected: no output. Any hit is a link still pointing at the removed `/radar/:topicId` route — fix it and re-run Step 1.

- [ ] **Step 4: Manual smoke test**

Start the app (`make dev-db` then `make dev-run`, and `npm run dev` in `web/`) and walk through:
1. `/radar` lists unread matches from more than one topic, newest first, each card stamped with its topic.
2. `All` shows previously-read matches; `New` hides them again; both states survive a page reload via the URL.
3. Clicking a topic chip filters the list; clicking it again clears the filter.
4. Opening a match and going back removes it from the `New` list and decrements its chip count.
5. `Topics →` reaches `/radar/topics`; a topic card opens `/radar/topics/:id`; `← Back to inbox` returns to `/radar`.

- [ ] **Step 5: Commit any fixes**

```bash
git add web/src
git commit -m "fix(radar): address inbox verification findings"
```

Skip this step if Steps 1–4 needed no changes.
