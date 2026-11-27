# Radar: ✓ / ✗ feedback on topics

**Status:** implemented, 2026-11-27.
**Spec:** [`2026-11-25-radar-topic-matching-research.md`](../specs/2026-11-25-radar-topic-matching-research.md),
recommendation 2. Builds on
[`2026-11-26-radar-hub-corrected-matching.md`](2026-11-26-radar-hub-corrected-matching.md).
The "name (what it is)" nudge is dropped: on the implemented score it breaks
plain concepts (see the spec's follow-up under recommendation 2). The writing
tips stay as they are.

## What the marks do

- **✓ "about this topic"** moves the topic towards the marked articles
  (Rocchio), so similar articles score higher. It also keeps the cutoff from
  rising above anything marked ✓.
- **✗ "not about this topic"** hides the article from the topic and raises
  the topic's cutoff just above it. Everything that scored as low as the
  rejected article goes with it.

The marks belong to the topic and survive edits of its description: the
topic is always learned again from description + marks.

## Measured

Same corpus as the research (17 topics, short and long phrasings), on the
implemented score. Each run splits the findings at random into a past half,
where a simulated user reviews the rows above the cutoff, and a future half,
where the learned topic is scored. Mean of 20 splits. Empty stream: nothing
about the topic in the feed; false alerts in the future half (~650
findings), summed over 17 short topics.

| User marks | F1 | Recall | FP / 1k | Empty stream |
|---|---|---|---|---|
| nothing | 0.896 | 0.888 | 1.22 | 12.4 |
| ✗ on the wrong rows of the top 5 (preview) | 0.894 | 0.886 | 1.22 | **5.1** |
| ✓ and ✗ on the top 5 | 0.920 | 0.939 | 1.52 | 5.1 |
| ✓ and ✗ on everything seen | **0.936** | 0.917 | **0.48** | 5.1 |
| ✗ only, on everything seen | 0.849 | 0.784 | 0.32 | — |

Two mechanisms were rejected:

- **Rejected articles as examples** (hide new findings closer to a rejected
  one than to any ✓). Noise is as close to a topic's real articles as to other
  noise (median cosine 0.47 vs 0.48), so it cannot tell them apart: at
  similarity 0.60 it costs recall (F1 0.809 with ✗ only) and barely touches
  the empty stream (12.4 → 9.9). Steam Frame is closer to some real Steam Deck
  articles (0.72) than most of them are to each other.
- **Plain Rocchio on the new score.** Moving the topic towards articles
  raises every article's score, so a fixed cutoff lets the stream in
  (α = 0.5: FP 1.22 → 9.4). Subtracting the shift of the stream's mean score
  (the offset below) fixes it.

The known cost is the last row: a user who only rejects and never confirms
loses recall, because a rejected article can score as high as real ones
(Steam Frame 0.29 = the 10th percentile of real Steam Deck articles). The UI
answers it by showing what falls below the cutoff so it can be ✓'d back,
and ✓ caps the cutoff.

## The learned topic

With `q` the description's embedding, `m` a finding's match vector,
`m̂ = m/|m|`, `c` the reference centroid, `t` the topic's `match_threshold`
(0.18) and `μ` the mean `m` of the last 2,000 findings:

    q'     = normalize(q + mean(m̂ of ✓ findings))        q' = q without ✓
    offset = μ · (q' − q)
    score  = m · (q' − c) − offset
    cutoff = max(t, max score of ✗ + 0.005)
    with ✓: cutoff = min(cutoff, max(t, min score of ✓ − 0.005))

The offset keeps the stream's mean score where it was, so 0.18 means the same
for every topic. It is a per-topic constant, so the preview still orders by
the index. α = 1 measured best (0.5: F1 0.928 vs 0.936 with everything seen).

## Steps

1. **Migration 019.**
   - `radar_topic_feedback (topic_id, finding_id, verdict, created_at)`,
     primary key `(topic_id, finding_id)`, both cascade on delete; `verdict`
     is `relevant` or `irrelevant`. Findings are never deleted (only their
     downloaded pages), so the marked vectors stay available.
   - `radar_topics.match_offset real NOT NULL DEFAULT 0` and
     `match_cutoff real NOT NULL`, set to `match_threshold` for existing
     topics. `match_threshold` stays the base the user sets (the future
     strictness slider); `match_cutoff` is derived.
2. **Store.**
   - Feedback: set, delete, list for a topic (with finding summaries), count.
   - `MatchFindingToTopics` scores `-(rt.match_embedding <#> f.match_embedding) - rt.match_offset`
     against `rt.match_cutoff` and skips findings marked ✗ for that topic.
   - `PreviewFindings` takes the offset and a list of finding ids to leave
     out; it subtracts the offset from the displayed score.
   - `StreamMean(ctx)`: `avg(match_embedding)` over the last 2,000 findings.
   - Match vectors of a set of findings, for learning.
3. **Learning** (`internal/radar/learn.go`): a pure function from `q`, the
   centroid, `μ`, `t` and the marked findings' vectors to
   `(match vector, offset, cutoff)`, unit-tested against the formulas above
   (✗ lifts the cutoff above it, ✓ caps it, the offset keeps `μ`'s score).
   The service calls it on create, on description edits, on every mark and
   unmark, and in the preview without saving. `reembed-topics` goes through
   it too, so it cannot drop what ✓ taught. `FillTopicMatchEmbeddings` only
   touches topics without a match vector, which never have marks.
4. **API.**
   - `POST /radar/topics/preview` and `POST /radar/topics` take
     `feedback: [{finding_id, verdict}]`; the preview leaves ✗'d findings
     out, marks ✓'d ones and returns the learned cutoff as `threshold`.
     `previewLimit` 5 → 10, so rows that drop below the cutoff stay visible.
   - `PATCH /radar/topics/{id}` takes `feedback` too, replacing the topic's
     marks (the edit dialog saves them with the description).
   - `PUT /radar/topics/{id}/feedback/{findingId}` `{verdict}` and `DELETE`
     for marks outside the dialogs. ✗ removes that topic's match of the
     finding; undoing it runs matching for the finding again.
   - `GET /radar/topics/{id}/feedback` lists the marks with finding titles;
     topics carry `match_cutoff` and mark counts.
5. **UI.**
   - Preview rows (new and edit dialogs) get ✓ / ✗. ✗ takes the row out and
     adds "N marked not about this · show" to undo; ✓ stays checked. The
     cutoff line moves with every mark.
   - The match reader gets "About this topic" / "Not about this topic"; ✗
     goes back to the list with an Undo toast.
   - The topic page shows "learned from N ✓ · M ✗" with "Forget marks".
6. **radar-sim** prints the offset and cutoff and scores topics with them.
7. Tests for each step; check on a copy of the dev database: an empty topic
   like "BlackArch Linux in Docker" goes quiet after ✗ on its preview rows,
   and a topic with real matches keeps them after ✓ on a few.

## As built

Where the implementation differs from the steps above:

- **✗ hides a match instead of deleting it.** Lists and counts leave out
  matches whose finding is marked irrelevant, and the matcher skips such
  findings; taking the mark back restores the match exactly as it was. No
  re-matching is needed for undo.
- **The match reader stays on the article.** It asks "Is this about
  <topic>?" under the header and, once answered, shows the verdict with
  Undo. Matches carry the user's verdict for that (`verdict` on match
  responses).
- **"Forget all" lives in the topic dialogs**, next to the list of marks
  (each with Undo); the topic page shows "learned from N ✓ · M ✗". The
  edit dialog gained the same preview as the new-topic dialog, starting
  from the saved marks, and saves marks only when they were changed.
- **`reembed-topics` goes through the service** (`Service.ReembedTopic`),
  so it relearns from the marks rather than resetting the topic.
- The stream mean is cached for 10 minutes and only read when a topic has
  ✓ marks. The preview still learns against the default threshold, as it
  did before.

Checked on a copy of the dev database (761 findings, 11 feeds) with TEI.
"self-hosting" starts with five rows above 0.18, two of them noise:
✗ on "The American Religion of Self-Storage Facilities" lifts the cutoff to
0.197, ✗ on "1984 Hosting is down" to 0.251; ✓ on the two real articles
moves them from 0.27–0.30 to 0.39–0.41 while the rest of the stream stays at
0.15–0.18. "BlackArch Linux in Docker" finds the English and Russian
versions of the same post (0.40, 0.37) with nothing else above 0.14, so the
server's empty-stream noise could not be reproduced here.

## Not in this step

- ✗ directly on match cards: the card is one link, so a button needs the
  card restructured. Next, if the reader buttons prove too slow to reach.
- Re-scoring existing matches when the topic learns: marks change what is
  matched from then on; matches already made stay unless ✗'d one by one.
- Hiding the same story from other feeds once it was ✗'d (near-duplicates);
  the research corpus has none to measure it on.
- The reranker and LLM features stay deferred.
