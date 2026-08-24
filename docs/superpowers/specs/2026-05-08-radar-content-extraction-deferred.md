# Radar full-content extraction (deferred)

**Status:** deferred — to be designed alongside reader view for findings.
**Decided:** 2026-05-08.

## Context

Today the Radar embedder builds its vector from `title + summary`, where
`summary` is the `<description>` from the RSS feed. Many aggregators (HN,
Lobsters, Reddit) put a placeholder there — `<a>Comments</a>`, a link to the
discussion, or just a URL. In phase 3a-2 such descriptions are stripped at the
crawler level (`sanitizeSummary` in `internal/radar/crawler/crawler.go`), and
for those feeds the embedding is built from the title alone.

That works, but topic-matching quality for aggregators stays limited: a title is
a headline, with none of the article's context. The full answer is a separate
pipeline step that follows `finding.url`, extracts the content, and feeds it to
the embedder.

The `radar_findings.content_id BIGINT NULL REFERENCES article_contents(id)`
column is already reserved for this (migration 009) and is always NULL for now.

## Target pipeline

```
Scheduler → CrawlFeed → FetchContent → EmbedFinding → MatchFinding
                          ↑ NEW
```

`FetchContent` takes `finding.url`, extracts the main text, saves it into
`article_contents`, and sets `radar_findings.content_id`. From then on
`EmbedFinding` prefers the text in `article_contents` over `summary`.

## What to do when we implement it

### 1. The `FetchFindingContent` job

A new river worker in `internal/radar/jobs/fetch_finding_content.go`. Argument:
`FindingID int64`. Steps:

1. Load the finding (for the url, and to check `content_id` is not set already).
2. HTTP GET with a timeout and a size limit (like `HTTPFetcher` in the crawler).
3. Run the HTML through an extractor (see below).
4. If content came out — `INSERT INTO article_contents` plus
   `UPDATE radar_findings SET content_id`.
5. Enqueue `EmbedFinding`. (`CrawlFeed` does that today; it has to move.)

### 2. The extractor

Options:

- **`go-readability`** (`github.com/go-shiori/go-readability`) — a Go port of
  Mozilla Readability, maintained, no CGO. Handles typical articles well.
- **`go-trafilatura`** — a port of trafilatura, better on news sites but noisier
  in its dependencies.
- **A naive custom strip** — no, too many cases (paywalls, cookie banners,
  JS-only pages).

Default: `go-readability`. The fallback when the extractor returns nothing: leave
`content_id = NULL` and let the embedder work from the title.

### 3. Robots.txt and rate limiting

Openly fetching URLs out of arbitrary RSS feeds reproduces the original DoS
problem (see `2026-05-06-user-added-feeds-deferred.md`), on top of the legal and
ethical considerations.

At minimum:

- Respect `robots.txt` (`github.com/temoto/robotstxt`, for instance), cached
  per host for a day.
- A per-host token bucket (no more than 1 RPS per host, say).
- A request timeout ≤ 15 seconds and a size limit ≤ 5 MiB.
- A User-Agent with contact details (the way Wayback and Common Crawl do it).

### 4. Storage

`article_contents` already exists, from Library. The schema is shared between
Library and Radar — both drop extracted text into the same table. Worth
checking:

- Is there a uniqueness constraint on url? If not, add one so the same URL is
  not extracted twice (once by Library, once by Radar).
- Content size: BLOB or TEXT? If there is no limit, add a CHECK on the maximum
  size (~1 MiB after cleanup).

### 5. Error handling

`FetchContent` must not take down the whole pipeline over a single failure.
The semantics:

- 4xx (404, 403) → a permanent error; do not retry, leave NULL, log it.
- 5xx / network → a river retry with backoff (the standard mechanism).
- robots.txt forbids it → neither a fetch nor an error — just leave NULL.
- The extractor returned nothing, or text that is too short → leave NULL.

### 6. The embedder

`internal/radar/jobs/embed_finding.go` — `embedText`. The new logic:

```
if finding.content_id != NULL:
    text = article_contents.content (capped at, say, 4096 tokens)
else:
    text = title + summary
```

The token cap is needed because bge-m3 has a limit of about 8192, and feeding
whole long articles is wasteful (TEI is slow, and the embedding gets noisy from
the volume of secondary content).

## When to come back

Come back to this document when reader view for findings gets built (phase 3b or
later): viewing needs the extracted text too, and doing the extraction twice
would be wasteful.

Until then, `sanitizeSummary` in the crawler is enough to keep the most obvious
noise out of aggregator feeds' embeddings.

## Related documents

- `docs/superpowers/specs/2026-05-06-embedding-model-decision.md` — the choice of
  bge-m3 and the token cap.
- `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` — quotas for
  user-contributed feeds (the same DoS story as here).
