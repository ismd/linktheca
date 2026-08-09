# Linktheca

Self-hosted read-it-later with semantic news monitoring.

Linktheca lets you save links and articles for later reading, and — independently — watches the web for new articles that match topics you care about. Both pieces run on your own server. No cloud services are required for core functionality.

## What it does

- **Library** — your read-it-later list. Articles you save, parsed and stored locally.
- **Radar** — semantic news monitoring. You describe topics in natural language; a crawler ingests feeds and surfaces articles that match each topic by embedding similarity, not keywords.

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

To inspect what Radar's embeddings actually score, run the `radar-sim` tool — it ranks findings by cosine similarity against a query you type or against a topic's stored embedding, and draws the match threshold as a cutoff line. It reads the same `LINKTHECA_DB_DSN` / `LINKTHECA_TEI_URL` variables as the backend:

```sh
go run ./cmd/radar-sim -topics                 # topic ids, thresholds, embedding state
go run ./cmd/radar-sim -q "webauthn passkeys"  # rank findings against fresh text
go run ./cmd/radar-sim -topic 3 -subscribed    # rank against a topic, matcher scoping
go run ./cmd/radar-sim -h                      # all flags
```

## Production

`compose.prod.yaml` brings up Postgres, the TEI embedding server, the Go backend, and the web frontend behind nginx. Set `POSTGRES_PASSWORD` and `JWT_SECRET` in the environment.

```sh
docker compose -f compose.prod.yaml up -d
```

## Status

Active development. APIs and schema may still change.

## License

[AGPL-3.0](LICENSE). If you run a modified version of Linktheca as a network service, you must offer its source code to your users.
