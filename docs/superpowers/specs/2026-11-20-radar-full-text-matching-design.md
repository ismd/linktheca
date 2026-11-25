# Radar: matching on the article, single-field topics

**Status:** implemented 2026-11-25.
**Supersedes:** [`2026-05-08-radar-content-extraction-deferred.md`](2026-05-08-radar-content-extraction-deferred.md).

## Why

Two things made Radar match badly.

1. **Findings had almost no text.** A finding was embedded from
   `title + "\n" + summary`, where the summary is the feed's `<description>`.
   Aggregators (HN, Lobsters, Reddit) put a "Comments" link there, which the
   crawler strips, so for them only the headline was left.
2. **Topics had two fields nobody knew how to fill.** Name and description
   were glued into `"Name: description"` and embedded together. Users could
   not tell what to write where, and renaming a topic silently left its
   embedding stale.

## Decisions

- **One vector per finding, from the title and the opening of the article**
  — not chunks with max-similarity. The schema, the matching SQL and the HNSW
  index stay as they were. Chunks remain an option if `radar-sim` shows
  articles missed because their topic only comes up late.
- **The opening is ~2000 characters (~450 tokens).** Cost decides it: on CPU,
  bge-m3 takes ~1.7 s for 2000 characters and ~4.5 s for the 1024 tokens TEI
  truncates to, and every finding goes through it. The opening is where an
  article says what it is about.
- **A topic is one field.** Only the description is embedded. The name is a
  display label: the backend derives it from the start of the description
  (first line, cut at the first `. ! ? : ;` or spaced dash, at most 60
  characters on a word boundary) and the user can rename it afterwards, which
  no longer touches the embedding.
- **The downloaded article is shown in the Radar reader**, sanitized like the
  Library's.
- **Learning from what users do with matches is not built**, but nothing here
  closes it off: finding embeddings outlive the article text, and the topic
  embedding column can stay the description's base vector.

## Pipeline

```
ScheduleCrawls → CrawlFeed → FetchFindingContent → EmbedFinding → MatchFinding
                                  radar_fetch          radar_embed
PruneFindingContent (daily)
```

`FetchFindingContent` (`internal/radar/jobs/fetch_finding_content.go`):

1. A page already in `article_contents` with enough text (another feed linked
   it, or someone saved it to their Library) is linked without a request.
2. `crawler.HostPacer` keeps requests to one host at least
   `LINKTHECA_RADAR_FETCH_HOST_INTERVAL` (2 s) apart. A job that has to wait
   snoozes, and its slot is held for it, so it waits once.
3. `crawler.Robots` follows robots.txt (RFC 9309 via `jimsmart/grobotstxt`,
   matched as `Linktheca`), cached per site for a day. A 4xx robots.txt allows
   everything; a 5xx is retried later.
4. The page is extracted with readability: 15 s, 5 MiB, HTML only. Less than
   300 characters of text counts as no article (paywall, cookie wall, a page
   that renders in JavaScript).
5. Whatever happens, the finding moves on to `EmbedFinding`. Transient
   failures (5xx, 429, timeouts) are retried twice first; anything else falls
   back at once to the title and summary. Failed downloads are not stored.

`LINKTHECA_RADAR_FETCH_CONTENT=false` sends findings straight to
`EmbedFinding`, as before.

Downloads and embeddings run in queues of their own. TEI on CPU works through
one or two long inputs at a time, so more parallel requests only pile up in
its queue until the client times out; `radar_embed` runs
`LINKTHECA_RADAR_EMBED_WORKERS` (2) at once.

## Fetching untrusted URLs

Any user can add a personal feed, so every URL Radar or the Library fetches is
untrusted. `internal/core/netsafe` builds the HTTP client for all of them
(feeds, pages, images, robots.txt): it refuses loopback, private, link-local,
CGNAT, multicast and reserved addresses after DNS resolution, on every
redirect hop, and follows at most 5 redirects. Environment proxies are honored
only with `LINKTHECA_FETCH_ALLOW_PRIVATE_NETWORKS=true`, which also turns the
checks off.

Article HTML is sanitized with bluemonday's UGC policy when it is served
(`content.SanitizeHTML`), in both readers. That also covers rows stored before
the sanitizer existed.

## Storage and retention

`article_contents` is shared. `content.Store` owns it; the Library store
delegates to it. A second fetch of a URL fills in what it found and keeps the
rest, a failure never hides text an earlier fetch extracted, and a success
clears an earlier error.

`PruneFindingContent` runs daily and deletes pages fetched more than
`LINKTHECA_RADAR_CONTENT_RETENTION` (14 days) ago that are linked only to
findings that matched no topic and are in nobody's Library. Migration 016 makes
`radar_findings.content_id` `ON DELETE SET NULL`: the finding keeps its title,
summary and embedding.

## Reader

`GET /radar/matches/{id}` returns `finding.content` (HTML, text, byline, site,
reading time) while the page is stored. Lists and topic previews fall back to
the page excerpt when the feed had no summary, which gives HN cards a
description. The frontend shares `ArticleBody` between the Library and Radar
readers.

## Rollout

- Migration 015 folds old topic names into their description where the
  description does not mention them, so those topics keep their meaning.
- `reembed-topics` recomputes topic embeddings; `radar-backfill-content`
  downloads the articles of recent findings and re-embeds them without
  matching them again. Both ship in the backend image.

## Threshold and text length

Measured on a copy of the dev database: 583 findings from HN, a security blog
and CTFtime, 443 of them with a downloaded article (the rest fell back), and
eight topics written the way the new dialog suggests. Relevant findings were
labeled by hand among each topic's candidates; for the CTF topic by title.
Every finding was scored twice, embedded from the feed's title and summary and
from the title and the article's opening.

- **The noise floor did not move.** Over all topic/finding pairs the median
  similarity is 0.38 either way, the 90th percentile 0.46.
- **Relevant findings move in two directions.** Items whose feed carried only
  a headline (HN) rise: on "LLM releases", relevant posts went from 0.31–0.50 to
  0.47–0.52 and from scattered through the list to its top. Items that already
  had a good summary drop a little: CTF writeups from 0.61–0.69 to 0.52–0.66.
- **0.55 is now too high.** Macro F1 over the topics falls from 0.52 (summary,
  0.55) to 0.36 (article, 0.55); with the article it peaks at 0.66 around
  0.50–0.52. At 0.52 every topic finds at least as much as before (CTF 86 →
  129 of 215, LLM 0 → 1 of 7, WebAuthn 1 → 2 of 2) for a few more false
  positives, mostly a blog's About and Contact pages. 0.51 finds more LLM posts
  but triples the false positives on the vulnerability topic, so the default
  is **0.52** (migration 017; topics still at 0.55 move with it).
- **The opening is enough.** Re-embedding the 189 candidate findings from
  8000 characters instead of 2000 gave a mean average precision of 0.687
  against 0.695, for 2.6× the TEI time.

The sample is small and the labels are one person's; `radar-sim -topic <id>`
shows where a given topic's matches fall, and the per-topic threshold is the
lever for topics that need a different cut.
