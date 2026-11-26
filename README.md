# Linktheca

Self-hosted read-it-later with semantic news monitoring.

Linktheca lets you save links and articles for later reading, and — independently — watches the web for new articles that match topics you care about. Both pieces run on your own server. No cloud services are required for core functionality.

## What it does

- **Library** — your read-it-later list. Articles you save, parsed and stored locally.
- **Radar** — semantic news monitoring. You describe topics in natural language; a crawler ingests feeds, downloads the article behind each item, and surfaces the ones that match a topic by embedding similarity, not keywords.

Library and Radar are deliberately kept separate: the things you saved don't get mixed with what the crawler found.

## How it works

- **Backend:** Go, Postgres (with [pgvector](https://github.com/pgvector/pgvector)).
- **Frontend:** React + TypeScript (Vite, TanStack Query, Tailwind).
- **Embeddings:** local [text-embeddings-inference](https://github.com/huggingface/text-embeddings-inference) running `BAAI/bge-m3`. No external embedding API.
- **LLM features** (optional, off by default) — opt-in, via cloud APIs.

Radar and any LLM features are guarded by independent flags; you can run Linktheca as a plain read-it-later if that's all you want.

## Quick start (dev)

```sh
make dev-db          # start Postgres in Docker
make dev-run         # run the backend
cd web && npm run dev
```

See `Makefile` for the full set of targets (tests, lint, build).

To inspect what Radar's embeddings actually score, run the `radar-sim` tool — it ranks findings by match score against a query you type or against a topic, prints the plain cosine alongside, and draws the match threshold as a cutoff line. It reads the same `LINKTHECA_DB_DSN` / `LINKTHECA_TEI_URL` variables as the backend:

```sh
go run ./cmd/radar-sim -topics                 # topic ids, thresholds, embedding state
go run ./cmd/radar-sim -q "webauthn passkeys"  # rank findings against fresh text
go run ./cmd/radar-sim -topic 3 -subscribed    # rank against a topic, matcher scoping
go run ./cmd/radar-sim -h                      # all flags
```

## Requirements

RAM is dominated by TEI, the embedding server; Postgres, the backend, nginx and the OS together fit in about 1 GB.

- **With Radar: 4 GB minimum, 8 GB recommended.** Tuned TEI peaks at ~3.1 GB, and it peaks while loading the model — at 4 GB configure swap, since a kill during load becomes a restart loop.
- **Without Radar: 1 GB minimum, 2 GB recommended.** With `LINKTHECA_RADAR_ENABLED=false` the backend never constructs a TEI client, so you can drop the `tei` service from compose entirely.

Disk: ~2.1 GB for the model, plus Postgres data (~10 KB per Radar finding, embedding and index included) and saved images (10 MB each at most).

TEI's own defaults assume a dedicated inference box — on a 22-core host it peaks at 14.2 GB. `compose.prod.yaml` pins two flags to get to 3.1 GB:

- `--max-batch-tokens=1024` — the CPU backend has no flash attention, so bge-m3's 8192-token limit means one request can materialize a ~4.3 GB attention matrix. This also clamps input length, which costs nothing here: a finding is embedded from its title and the first ~2000 characters of the article (~450 tokens).
- `--tokenization-workers=2` — defaults to the host's core count, and each worker keeps its own ~210 MB copy of the tokenizer.

## Production

`compose.prod.yaml` brings up Postgres, the TEI embedding server, the Go backend, and the web frontend behind nginx. Set `POSTGRES_PASSWORD` and `JWT_SECRET` in the environment.

```sh
docker compose -f compose.prod.yaml up -d
```

See `.env.example` for every setting. Two worth knowing about:

- Pages, feeds and images are fetched only from public addresses, so a link cannot make the server probe its own network. If you read feeds or save pages from your LAN or tailnet, set `LINKTHECA_FETCH_ALLOW_PRIVATE_NETWORKS=true`.
- Radar downloads every article its feeds link to, following robots.txt and spacing requests to one site by `LINKTHECA_RADAR_FETCH_HOST_INTERVAL`. Articles that matched none of your topics are deleted after `LINKTHECA_RADAR_CONTENT_RETENTION` (14 days). `LINKTHECA_RADAR_FETCH_CONTENT=false` turns downloading off and matches on the feed's title and summary instead.

### Maintenance commands

The backend image ships two one-off commands next to the server. They read the same environment, so run them inside the running container:

```sh
# Recompute every topic's embedding, after the way topics are embedded changes.
docker compose -f compose.prod.yaml exec backend reembed-topics

# Download the articles of findings from the last N days and re-embed them.
# Queues jobs for the running server; add -match to match them against topics again.
docker compose -f compose.prod.yaml exec backend radar-backfill-content -days 7
```

### Upgrading to full-text Radar

Releases from November 2026 change what Radar compares: a topic is embedded from its description alone, and a finding from the article instead of the feed's summary. After upgrading, run `reembed-topics` once, then `radar-backfill-content` so that recent findings are scored the same way as new ones. Topics still at the old default threshold move to the new one automatically.

From migration 018 on, a finding is also scored against its title, and findings that resemble every topic are discounted. Nothing needs to be run by hand: the server fills in the new vectors for existing topics at start and embeds the titles of existing findings in the background, newest first and behind new findings. Until a finding's title is embedded it does not show up in topic previews. Thresholds move to the new scale automatically.

## Status

Active development. APIs and schema may still change.

## License

[AGPL-3.0](LICENSE). If you run a modified version of Linktheca as a network service, you must offer its source code to your users.
