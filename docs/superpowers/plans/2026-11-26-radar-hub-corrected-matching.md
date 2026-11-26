# Radar: hub-corrected matching with title fusion

**Status:** implemented, 2026-11-26.
**Spec:** [`2026-11-25-radar-topic-matching-research.md`](../specs/2026-11-25-radar-topic-matching-research.md),
recommendation 1. The reranker (3) and the LLM gloss stay deferred; the dialog
tip and ✓/✗ feedback (2) come later.

## What changes, and why not the per-topic z-score

The research recommended three parts: a title embedding, a hub correction and
per-topic background statistics (z-score). Re-checked with the same LOTO
protocol while planning, the first two alone with one global threshold do as
well, with no statistics to keep:

| Rule | F1 (short+long) | FP / 1k | Empty stream, short | LLMs / Security recall | MAP |
|---|---|---|---|---|---|
| cos ≥ 0.525 (today) | 0.785 | 3.3 | 40 | 0.25 / 0.23 | 0.911 |
| fused ≥ 0.42 and z ≥ 3.5 | 0.889 | 1.7 | 49 | 0.10 / 0.13 | — |
| **fused − hub ≥ 0.175** | **0.897** | **1.4** | **28** | 0.17 / 0.21 | **0.962** |

No quota effect on broad topics, nothing to compute per topic, no background
sample to wait for on a fresh instance. Weighting title vs article (0.4–0.6)
or the hub term (0.75–1.25) differently moves F1 by under 0.005, so the
measured 0.5 / 1 stay.

## The score

With `d` the article embedding, `t` the title embedding, `q` the topic
embedding and `c` the mean of 100 reference topic embeddings:

    score = (q·d + q·t)/2 − (c·d + c·t)/2 = (q − c) · (d + t)/2

So each finding stores `m = (d + t)/2` and each topic `q' = q − c`, and the
score is their inner product: indexable with HNSW `vector_ip_ops`, so the
preview stays an index scan. Threshold 0.18 (LOTO picked 0.175; 0.18 has the
same F1, fewer false alerts, and displays cleanly at two decimals).

## Steps

1. **Migration 018.** `match_embedding vector(1024)` on findings and topics;
   HNSW `vector_ip_ops` index on findings' replaces the cosine index on
   `embedding`; `match_threshold` default 0.18, untuned 0.52 → 0.18, tuned
   values shifted by −0.34. `radar_topic_matches.similarity` holds the new
   score from here on.
2. **Reference topics** (`internal/radar/reference.go`): the 100 phrases from
   the research, centroid embedded lazily via TEI and cached per process;
   `TopicMatchVector(q)` and `FindingMatchVector(d, t)`.
3. **Embedding.** `EmbedFindingWorker` also embeds the title (skipped when the
   whole embed text is the title) and stores `m`. Topics store `q'` next to
   `q` on create, edit and in `reembed-topics`.
4. **Matching and preview** score with `-(q' <#> m)` against
   `match_threshold`; preview ranks by it.
5. **Backfill**, automatic: a periodic job (every 10 minutes and on start) fills `q'` for
   topics from their stored `q`, and queues low-priority title-embedding jobs
   for findings without `m`, newest first.
6. **radar-sim** prints the score and the cosine side by side.
7. Tests for each step; check on the dev instance.

   On a copy of the dev database (650 findings, two topics) the backfill took
   95 s through TEI on CPU. Every match the old rule made on a real article
   was kept; the noise it let through went ("Hacking OpenAI" for
   "WebAuthn и PassKeys" at cos 0.56, score 0.12). Probes: "Rust" keeps its
   four Rust articles and drops a Clojure/Lean one at cos 0.53; "Steam Deck"
   matches nothing, as there is nothing about it; "self-hosting" still lets
   "The American Religion of Self-Storage Facilities" through (0.19), as the
   old rule did.

## Caveats carried over

English-only evaluation; the reference topics are English, and how they
correct non-English findings is untested. Thresholds come from one corpus.
