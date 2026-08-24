# Per-topic match threshold slider with similarity preview (deferred)

**Status:** deferred — to be designed when Radar topic management UI gets built.
**Decided:** 2026-05-08.

## Context

`radar_topics.match_threshold` is already per-topic (REAL NOT NULL DEFAULT 0.55).
Right now the user has no UI control over it: a topic is created with the
default, and the user has no idea which findings will land in their Radar until
something — or nothing — shows up.

Dev data shows empirically that **different topics need different thresholds**,
because BGE-M3's cosine similarity is squeezed into [0.4, 0.8] and the shape of
the distribution depends on how broad the topic is:

| Topic (description) | Noise ceiling | Real matches |
|---|---|---|
| AI ("machine learning research and large language models") | ~0.48 | 0.45–0.55 (Gemma, LLM benchmarks, AI agents) |
| WebAuthn ("webauthn passkeys") | ~0.49 | 0.59 (pinpoint matches) |
| Wolfenstein ("Wolfenstein 3D for Gameboy") | ~0.32 | 0.75 (direct lexical) |

Broad topics (AI) produce many borderline cases in [0.5, 0.55), and the user
probably wants a threshold of 0.50.
Narrow terms (WebAuthn) are pure noise below 0.55, so 0.55+ is the right call.
Lexical matches (Wolfenstein) are insensitive to the threshold.

Without visual feedback a user cannot guess the "right" threshold — a sim of
0.55 means nothing without the context of the distribution over their own data.

## Target UX

A "Match threshold" slider (0.40–0.90, step 0.01 or 0.05) on the topic editing
page. Beside it, a live preview listing the top-N findings with their sim
scores, ordered descending. Dragging the slider makes it visible what drops out
and what comes in:

```
Match threshold: ▓▓▓▓▓▓▓▓░░░░ 0.55

Would match (current setting):
  0.75  Wolfenstein 3D for Gameboy Color on custom cartridge (2016)
  0.59  WebAuthn and Passkeys — authentication without passwords
  ───────────────────────────── threshold ─────────────────────────────
  0.54  A Theory of Deep Learning                       (would NOT match)
  0.53  ProgramBench: Can Language Models Rebuild...    (would NOT match)
  0.50  Show HN: Adam – embeddable AI agent library     (would NOT match)
  0.48  Three Inverse Laws of AI                        (noise zone)
  0.47  Telus Uses AI to Alter Call-Agent Accents
  ...
```

That turns an abstract "0.55 vs 0.50" into a concrete "these four articles come
in if I lower the threshold".

## What to do when we implement it

### 1. Backend endpoint

`GET /radar/topics/{id}/preview?limit=20` returns the top-N findings sorted by
sim against the topic's embedding:

```json
{
  "findings": [
    {"id": 18, "title": "...", "sim": 0.7497, "would_match": true},
    {"id": 452, "title": "...", "sim": 0.5933, "would_match": true},
    {"id": 275, "title": "...", "sim": 0.5380, "would_match": false},
    ...
  ],
  "current_threshold": 0.55
}
```

The SQL is the same expression as in `MatchFindingToTopics`, minus the
`match_threshold` filter and with a LIMIT:

```sql
SELECT f.id, f.title, 1 - (rt.embedding <=> f.embedding) AS sim
FROM radar_topics rt
JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
JOIN radar_findings f ON f.feed_id = rfs.feed_id
WHERE rt.id = $1
  AND rt.embedding IS NOT NULL AND f.embedding IS NOT NULL
ORDER BY sim DESC
LIMIT $2;
```

Cost: one top-N query per drag is far too often. Cache the list on the client
and filter it in memory as the slider moves; query only when the topic changes
or the page opens.

### 2. Frontend

The slider plus a list with a "threshold cutoff line" divider. As the slider
moves, the divider slides through the list and findings above and below are
highlighted. On "Save", `PATCH /radar/topics/{id}` with the new threshold.

### 3. Optional extension: tier guidance

Use the distribution data to suggest "recommended" zones right on the slider:

- a green zone (high precision): above the peak of the true positives;
- a yellow zone (grey area): where noise starts mixing in;
- a red zone (high recall, many false positives).

That needs an understanding of where a given topic's noise ceiling sits —
empirically about p95 or p98 of the sim distribution across all of that topic's
findings.

## Dependencies

- A topic editing UI at all (topics are not in the UI today, only in the admin
  API).
- Enough findings in the system for the preview to be informative (a slider over
  five findings is useless).

## When to come back

When:
1. A "My Topics" page exists (phase 3b or later), and
2. A typical user has at least ~50–100 findings, so the sim distribution is
   visible.

Until then users can change the threshold through a direct PATCH on the API (if
one gets added) or by hand via SQL in dev setups. The sim distribution itself is
visible through `go run ./cmd/radar-sim -topic <id>` — the CLI prints the same
"what is above and below the threshold" picture as the target slider, just
without the interactivity.

## Related documents

- `docs/superpowers/specs/2026-05-06-embedding-model-decision.md` — the choice of
  bge-m3 and its compressed cosine-similarity range, which is what makes a
  unified default impossible.
- `internal/radar/service.go` — `defaultMatchThreshold = 0.55` as the starting
  point.
- `internal/radar/store.go` `MatchFindingToTopics` — the matching SQL the preview
  query is built from.
- `cmd/radar-sim/queries.go` `topFindingsByTopic` — this preview query is already
  written and covered by tests for the CLI; the endpoint in item 1 only has to
  wrap it in an HTTP layer with `user_id` scoping.
- `POST /radar/topics/preview` (issue #8) — the neighbouring endpoint: it scores
  findings against a **draft** topic while the user types the description, and
  draws the same cutoff line at `defaultMatchThreshold`. Its
  `Store.PreviewFindings` already scopes by the user's subscriptions; the
  endpoint in item 1 only needs to take the embedding from `radar_topics` instead
  of a fresh probe.
