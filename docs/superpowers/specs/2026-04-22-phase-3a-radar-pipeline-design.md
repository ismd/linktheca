# Phase 3a: Radar Pipeline Backend — дизайн

**Дата:** 2026-04-22
**Статус:** approved, готов к writing-plans
**Предшественники:** phase 1 (auth, завершена), phase 2 (library backend, завершена)

## Контекст

После phase 2 Linktheca имеет рабочий backend read-it-later: auth flow + Library CRUD. Phase 3a добавляет **backend модуля Radar** — мониторинг новостей по подписанным темам через локальные embeddings.

Scope phase 3a намеренно урезан до «pipeline живой end-to-end» без полного HTTP CRUD. Это выделяет самый рискованный технический кусок (TEI + pgvector + River job queue) в отдельную итерацию. Полный HTTP API (List/Update/Delete topics и feeds, user-facing `/radar/feed`, reader view для findings) переезжает в phase 3b.

Основные архитектурные решения, закреплённые в ходе brainstorming:
1. **TEI вместо Ollama** — embedding-сервер HuggingFace Text Embeddings Inference, модель bge-m3 (1024 dim). Ollama отклонена: для embeddings-only задачи TEI быстрее и проще в эксплуатации. Будущие LLM-фичи пойдут через cloud API, не self-hosted, с отдельными opt-in флагами.
2. **CLI всегда работает через HTTP** — никакого прямого доступа к БД из CLI. В phase 3a CLI получает минимум POST endpoints для bootstrap'а (topics/feeds/subscriptions), остальные endpoints появятся в phase 3b.
3. **Два бинаря:** существующий `cmd/linktheca` переименовывается в `cmd/linktheca-server`; новый `cmd/linktheca` — CLI-инструмент с cobra для remote production use.
4. **Radar опционален** — `LINKTHECA_RADAR_ENABLED` (default true) контролирует backend-часть фичи. Когда false, сервис работает как обычный read-it-later без TEI.

## 1. Цель и scope

**Цель phase 3a:** backend pipeline Radar работает end-to-end на реальных RSS-данных. Админ через CLI заводит feed, юзер через CLI создаёт topic и подписку. Дальше River-воркеры периодически тянут RSS, эмбеддят находки через TEI, матчат их с темами по cosine similarity. Результаты лежат в `radar_topic_matches`, доступны через прямой SQL (HTTP-endpoint для чтения — phase 3b).

**В scope:**
- 5 миграций: `radar_topics`, `radar_feeds`, `radar_feed_subscriptions`, `radar_findings`, `radar_topic_matches` (номера 006–010; pgvector extension уже есть в миграции 001)
- Пакет `internal/core/embeddings/` — HTTP-клиент к TEI + `FakeEmbedder` для тестов
- Пакет `internal/radar/` с `types.go`, `store.go`, `service.go`, минимальным `http.go` (3 POST handler'а)
- Под-пакеты `internal/radar/crawler/` (gofeed-обёртка) и `internal/radar/jobs/` (River-воркеры: Scheduler, CrawlFeed, EmbedFinding, MatchFinding)
- Новый бинарь `cmd/linktheca/` (CLI) с подкомандами `auth *` и `radar *`
- Переименование `cmd/linktheca/` → `cmd/linktheca-server/`
- Env-конфиг: `LINKTHECA_TEI_URL`, `LINKTHECA_TEI_TIMEOUT`, `LINKTHECA_EMBEDDING_DIM`, `LINKTHECA_RADAR_ENABLED`, `LINKTHECA_RADAR_SCHEDULER_INTERVAL`, `LINKTHECA_RADAR_MAX_WORKERS`, `LINKTHECA_URL`
- `compose.dev.yaml` пополняется сервисом `tei`
- Smoke-тест под `-tags=smoke` с реальным TEI-контейнером

**Вне scope (phase 3b или позже):**
- HTTP endpoints `GET /radar/topics`, `GET /radar/feeds`, `GET /radar/feed`, `GET /radar/findings/:id/read`, Update/Delete на topics/feeds/subscriptions
- CSV/OPML-импорт в CLI
- Fetch полного контента статей по требованию, reader view
- Уведомления любого типа
- Feed auto-discovery
- Admin UI, prod docker-compose, production deployment story

## 2. Layout файлов

```
linktheca/
├── cmd/
│   ├── linktheca-server/         # переименование из cmd/linktheca/
│   │   └── main.go
│   └── linktheca/                # новый: CLI
│       ├── main.go               # регистрация cobra root
│       └── internal/
│           └── cli/
│               ├── root.go
│               ├── session/
│               │   ├── session.go          # ~/.config/linktheca/session.json
│               │   └── session_test.go
│               ├── apiclient/
│               │   ├── client.go           # HTTP с auto-refresh
│               │   ├── client_test.go
│               │   ├── auth.go             # auth endpoints
│               │   └── radar.go            # radar endpoints
│               ├── output/
│               │   ├── format.go           # table/json printers
│               │   └── format_test.go
│               └── cmd/
│                   ├── auth_register.go
│                   ├── auth_login.go
│                   ├── auth_logout.go
│                   ├── auth_whoami.go
│                   ├── radar_topic_add.go
│                   ├── radar_feed_add.go
│                   └── radar_subscribe.go
│
├── migrations/
│   ├── 006_radar_topics.sql
│   ├── 007_radar_feeds.sql
│   ├── 008_radar_feed_subscriptions.sql
│   ├── 009_radar_findings.sql
│   └── 010_radar_topic_matches.sql
│
├── internal/
│   ├── core/
│   │   └── embeddings/
│   │       ├── client.go                   # Client интерфейс + TEIClient
│   │       ├── client_test.go              # unit
│   │       ├── fake.go                     # FakeEmbedder
│   │       ├── fake_test.go
│   │       └── client_smoke_test.go        # //go:build smoke
│   ├── radar/
│   │   ├── types.go                        # Topic, Feed, Subscription, Finding, Match, DTOs
│   │   ├── store.go                        # SQL для 5 таблиц
│   │   ├── store_test.go                   # integration с testdb + pgvector
│   │   ├── service.go                      # CreateTopic, AddFeed, Subscribe
│   │   ├── service_test.go                 # unit с mock store + FakeEmbedder
│   │   ├── http.go                         # 3 POST handlers
│   │   ├── http_test.go                    # unit (validation)
│   │   ├── integration_test.go             # HTTP → service → store end-to-end
│   │   ├── crawler/
│   │   │   ├── crawler.go                  # Fetcher интерфейс + gofeed
│   │   │   └── crawler_test.go             # synthetic RSS, etag
│   │   └── jobs/
│   │       ├── jobs.go                     # River setup, args-типы
│   │       ├── crawl_feed.go
│   │       ├── embed_finding.go
│   │       ├── match_finding.go
│   │       ├── scheduler.go
│   │       ├── jobs_test.go                # integration с реальной БД
│   │       └── smoke_test.go               # //go:build smoke
│   └── server/
│       └── server.go                       # MODIFIED: TEI client, River client, radar wiring
│
├── compose.dev.yaml                         # MODIFIED: + сервис tei
├── Makefile                                 # MODIFIED: smoke-radar, server-build, cli-build
└── embeds.go                                # MODIFIED: embed новые миграции
```

**Ключевые решения layout'а:**

- **CLI-специфичные пакеты под `cmd/linktheca/internal/cli/`** (не в верхнеуровневом `internal/`). Backend-код не должен транзитивно тянуть cobra/viper. Go-конвенция: CLI-tool-specific код рядом с cmd'ом.
- **`jobs/` как отдельный под-пакет `internal/radar/`** — чтобы `service.go` не втаскивал зависимость от River. Service — чистая бизнес-логика, jobs — оркестрация.
- **`http.go` в phase 3a присутствует**, но содержит только 3 POST handler'а (~100-150 строк). Полный CRUD — phase 3b.
- **Миграции нумеруются с 006**, продолжая последовательность. Миграция pgvector extension уже выполнена в `001_init.sql`, дублировать не нужно.

## 3. Миграции 006–010

Conventions: `BIGINT GENERATED ALWAYS AS IDENTITY` (users.id — `INT`), `user_id INT REFERENCES users(id) ON DELETE CASCADE`, именованные индексы.

### 006_radar_topics.sql

```sql
-- +goose Up
CREATE TABLE radar_topics (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    embedding       vector(1024),
    match_threshold REAL NOT NULL DEFAULT 0.75,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_topics_user_active_idx ON radar_topics (user_id) WHERE is_active;

-- +goose Down
DROP TABLE radar_topics;
```

`embedding` — nullable: строка создаётся в два шага (INSERT → TEI-вызов → UPDATE), чтобы не держать транзакцию на время HTTP-запроса.

### 007_radar_feeds.sql

```sql
-- +goose Up
CREATE TABLE radar_feeds (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url                    TEXT NOT NULL UNIQUE,
    kind                   TEXT NOT NULL DEFAULT 'rss'
                           CHECK (kind IN ('rss', 'atom')),
    title                  TEXT,
    last_fetched_at        TIMESTAMPTZ,
    last_error             TEXT,
    etag                   TEXT,
    last_modified          TEXT,
    fetch_interval_seconds INT NOT NULL DEFAULT 3600,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_feeds_active_fetched_idx ON radar_feeds (is_active, last_fetched_at);

-- +goose Down
DROP TABLE radar_feeds;
```

Feeds глобальные (без `user_id`) — один RSS парсится один раз для всех подписчиков.

### 008_radar_feed_subscriptions.sql

```sql
-- +goose Up
CREATE TABLE radar_feed_subscriptions (
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id    BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feed_id)
);

-- +goose Down
DROP TABLE radar_feed_subscriptions;
```

### 009_radar_findings.sql

```sql
-- +goose Up
CREATE TABLE radar_findings (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id       BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    content_id    BIGINT REFERENCES article_contents(id),
    external_id   TEXT,
    url           TEXT NOT NULL,
    title         TEXT,
    summary       TEXT,
    embedding     vector(1024),
    published_at  TIMESTAMPTZ,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (feed_id, external_id)
);
CREATE INDEX radar_findings_discovered_idx ON radar_findings (discovered_at DESC);
CREATE INDEX radar_findings_embedding_hnsw ON radar_findings USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE radar_findings;
```

`content_id` nullable: при обнаружении в feed'е не парсим полный контент, только метаданные. Полный `article_contents` появится только когда юзер откроет находку (phase 3b).

HNSW-индекс для kNN-поиска в обратную сторону (phase 3b: «дай похожие findings к теме»). Для MatchJob (phase 3a: «дай все темы, близкие к одной finding»), индекс не используется, но лежит заранее.

### 010_radar_topic_matches.sql

```sql
-- +goose Up
CREATE TABLE radar_topic_matches (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic_id   BIGINT NOT NULL REFERENCES radar_topics(id) ON DELETE CASCADE,
    finding_id BIGINT NOT NULL REFERENCES radar_findings(id) ON DELETE CASCADE,
    similarity REAL NOT NULL,
    state      TEXT NOT NULL DEFAULT 'new'
               CHECK (state IN ('new', 'seen')),
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic_id, finding_id)
);
CREATE INDEX radar_topic_matches_topic_state_idx ON radar_topic_matches (topic_id, state, matched_at DESC);

-- +goose Down
DROP TABLE radar_topic_matches;
```

### River-миграции

Отдельным шагом при старте `linktheca-server` вызывается `rivermigrate.New(pool, nil).Migrate(ctx, rivermigrate.DirectionUp, nil)` — это создаёт таблицы `river_job`, `river_leader`, `river_queue`, `river_migration`. Версионирование River не пересекается с goose.

Порядок старта: goose → River migrator → запуск River client → HTTP сервер.

## 4. TEI интеграция

### Docker Compose — `compose.dev.yaml`

Добавляется сервис `tei`:

```yaml
services:
  postgres:
    # существующий сервис, не меняется

  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
    command: --model-id BAAI/bge-m3 --port 8080
    ports:
      - "8081:8080"                # хост-порт 8081 во избежание конфликта с backend
    volumes:
      - tei-data:/data             # кеш модели (~2.3 GB)
    healthcheck:
      test: ["CMD", "curl", "-fs", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 120s           # первый старт тянет модель из HF
    restart: unless-stopped

volumes:
  postgres-data:
  tei-data:
```

Backend в dev-режиме запускается вне compose (как сейчас phase 1/2): `make dev-server` с `LINKTHECA_TEI_URL=http://localhost:8081`.

Production-compose — будущая фаза, не в phase 3a.

### Пакет `internal/core/embeddings/`

**Интерфейс и реализация TEI:**

```go
package embeddings

type Client interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}

type TEIClient struct {
    baseURL    string
    httpClient *http.Client
}

func NewTEIClient(baseURL string, timeout time.Duration) *TEIClient {
    return &TEIClient{
        baseURL:    strings.TrimSuffix(baseURL, "/"),
        httpClient: &http.Client{Timeout: timeout},
    }
}

// POST {baseURL}/embed {"inputs":"text"} → [[f32, ...]]
// Возвращает первый массив.
func (c *TEIClient) Embed(ctx context.Context, text string) ([]float32, error) { /* ... */ }
```

**FakeEmbedder для тестов:**

```go
type FakeEmbedder struct {
    Dim int
}

// Детерминированно: SHA-256(text) раскатывается в Dim элементов, нормализуется по L2.
// Одинаковый текст → одинаковый вектор. Разные тексты далеки по cosine.
func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) { /* ... */ }
```

**Smoke-тест:**

```go
//go:build smoke

// Поднимает TEI testcontainer с bge-m3, делает Embed, проверяет len == 1024
// и что разные тексты дают разные векторы.
```

### Интеграция с pgvector

Используем `github.com/pgvector/pgvector-go`:

```go
import "github.com/pgvector/pgvector-go"

vec, _ := teiClient.Embed(ctx, description)
_, err := pool.Exec(ctx,
    `UPDATE radar_topics SET embedding = $1, updated_at = now() WHERE id = $2`,
    pgvector.NewVector(vec), topicID)
```

`pgx` сериализует `pgvector.Vector` в колонку `vector(1024)`.

### Валидация при старте

`EMBEDDING_DIM = 1024` фигурирует в конфиге. При старте сервера (если `RADAR_ENABLED=true`) делаем `embedder.Embed(ctx, "ping")`, проверяем `len(vec) == EMBEDDING_DIM`. При несоответствии — warning в лог, но не fail-fast (TEI может быть временно недоступен; сервер стартует, `/radar/*` endpoints потом вернут 503 на create-topic).

### Ограничение размерности

Размерность `vector(1024)` зашита в миграциях 006 и 009. Смена модели требует:
1. Новая миграция, меняющая размерность колонок.
2. Пересчёт **всех** `radar_topics.embedding` и `radar_findings.embedding`.
3. Пересоздание HNSW-индекса.

Это долгосрочное архитектурное решение, не в scope phase 3a.

## 5. Pipeline: River + jobs

Четыре типа job'ов:

```
SchedulerJob (periodic, каждые 5 минут)
    └─► CrawlFeedJob (один на feed)
            └─► EmbedFindingJob (один на новую finding)
                    └─► MatchFindingJob (один на finding с embedding'ом)
```

Все workers запускаются в том же процессе, что и HTTP-сервер (один бинарь).

**Wiring:** River client создаётся в `server.go` (DI-слой, владеет pool'ом). Набор workers и periodic jobs формируется в пакете `internal/radar/jobs` (функция вроде `jobs.RegisterWorkers(workers *river.Workers, service *radar.Service, embedder embeddings.Client)`), и `server.go` передаёт получившийся `*river.Workers` в `river.Config`.

### SchedulerJob

River поддерживает periodic jobs нативно (`river.PeriodicJob`). Регистрация при старте в `server.go`:

```go
riverClient, _ := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.RadarMaxWorkers}},
    Workers: workers,
    PeriodicJobs: []*river.PeriodicJob{
        river.NewPeriodicJob(
            river.PeriodicInterval(cfg.RadarSchedulerInterval),
            func() (river.JobArgs, *river.InsertOpts) {
                return jobs.ScheduleCrawlsArgs{}, nil
            },
            &river.PeriodicJobOpts{RunOnStart: true},
        ),
    },
})
```

Worker `ScheduleCrawlsWorker` внутри:

```sql
SELECT id FROM radar_feeds
WHERE is_active
  AND (last_fetched_at IS NULL
       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
LIMIT 100;
```

Для каждого id — `client.Insert(ctx, CrawlFeedArgs{FeedID: id}, nil)`.

### CrawlFeedJob

Аргумент: `FeedID int64`. Flow:

1. `SELECT * FROM radar_feeds WHERE id = $1` — достаёт url, etag, last_modified.
2. HTTP GET с `If-None-Match` и `If-Modified-Since`. 304 → `UPDATE last_fetched_at = now(), last_error = NULL`, выход.
3. Парсинг через `mmcdole/gofeed.Parser{}.Parse(reader)`.
4. Для каждого item:
   ```sql
   INSERT INTO radar_findings (feed_id, external_id, url, title, summary, published_at)
   VALUES ($1, $2, $3, $4, $5, $6)
   ON CONFLICT (feed_id, external_id) DO NOTHING
   RETURNING id;
   ```
   Для каждой возвращённой id — `client.Insert(ctx, EmbedFindingArgs{FindingID: id}, nil)`.
5. `UPDATE radar_feeds SET last_fetched_at = now(), etag = $1, last_modified = $2, last_error = NULL`.

Ошибки сети/парсинга: `UPDATE radar_feeds SET last_error = $1, last_fetched_at = now()`, возврат `error` → River ретраит с exponential backoff (дефолт 25 попыток за ~24 часа).

В этом шаге **НЕ парсим полный контент статьи** — только метаданные из RSS. Полный `article_contents` создаётся в phase 3b при открытии находки.

### EmbedFindingJob

Аргумент: `FindingID int64`. Flow:

1. `SELECT embedding, title, summary FROM radar_findings WHERE id = $1`.
2. Если `embedding IS NOT NULL` — выход (идемпотентность при повторах).
3. `text := strings.TrimSpace(title + "\n" + summary)`. Если пусто — завершить без ошибки.
4. `vec, err := embedder.Embed(ctx, text)`. Ошибка TEI → возврат error → River ретраит.
5. `UPDATE radar_findings SET embedding = $1 WHERE id = $2` с `pgvector.NewVector(vec)`.
6. `client.Insert(ctx, MatchFindingArgs{FindingID: id}, nil)`.

Эмбеддим **title + summary, не полный текст**. Экономия + для короткой новости достаточно.

### MatchFindingJob

Аргумент: `FindingID int64`. Один SQL:

```sql
INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
SELECT rt.id,
       $1,
       1 - (rt.embedding <=> f.embedding) AS similarity,
       'new'
FROM radar_topics rt
JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
JOIN radar_findings f ON f.id = $1
WHERE rfs.feed_id = f.feed_id
  AND rt.is_active
  AND rt.embedding IS NOT NULL
  AND 1 - (rt.embedding <=> f.embedding) >= rt.match_threshold
ON CONFLICT (topic_id, finding_id) DO NOTHING;
```

`<=>` — cosine distance от pgvector. Similarity = `1 - distance`. Матчинг против **всех подписанных тем всех подписанных юзеров** за один запрос.

### Idempotency и recovery

- **Перезапуск backend'а во время работы:** River persist'ит job'ы в Postgres, после рестарта подхватываются.
- **Повторное выполнение job'а:** `ON CONFLICT DO NOTHING` на INSERT'ах, `embedding IS NOT NULL` проверка перед embed'ингом.
- **Rate limiting для TEI:** `RadarMaxWorkers = 5` по умолчанию — максимум 5 параллельных embedding-запросов.

### Интерфейсы для тестирования

```go
// internal/radar/crawler/crawler.go
type Fetcher interface {
    Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error)
}
type FetchResult struct {
    StatusCode   int
    Body         []byte
    Etag         string
    LastModified string
    NotModified  bool
}
```

В тестах `CrawlFeedJob`: fake `Fetcher` отдаёт подготовленный RSS XML, `FakeEmbedder`, реальная БД через testdb.

### Принятые компромиссы

- Нет дедупликации похожих findings между feeds (одна новость из 5 RSS появится 5 раз).
- Нет rate limiting на TEI за пределами `MaxWorkers`.
- SchedulerJob берёт 100 feeds за цикл — достаточно для self-host.
- Нет smart-backoff для битых feeds (ретраит до упора, оператор сам чинит).

## 6. HTTP endpoints

### Маршруты

```
POST /radar/topics           RequireUser         создаёт тему, считает embedding
POST /radar/feeds            RequireAdmin        регистрирует глобальный feed
POST /radar/subscriptions    RequireUser         подписывает authed user
```

Все остальные endpoints Radar (List/Update/Delete, `GET /radar/feed`, `GET /radar/findings/:id/read`) — phase 3b.

### `POST /radar/topics`

Request:
```json
{
  "name": "ИИ и стартапы",
  "description": "Привлечение инвестиций, запуск продуктов и бенчмарки фундаментальных моделей.",
  "match_threshold": 0.75
}
```

Validation:
- `name` — 1..200 символов, required.
- `description` — 10..2000 символов, required.
- `match_threshold` — optional, [0.0, 1.0], default 0.75.

Handler flow:
1. Валидация через `go-playground/validator`.
2. `radarService.CreateTopic(ctx, userID, dto)`.
3. Service: INSERT → `embedder.Embed(ctx, description)` синхронно → UPDATE embedding.
4. Возврат `201 Created` + JSON topic object (без raw embedding — только `has_embedding: true`).

Почему embedding синхронно в handler'е: юзер ожидает ready-to-match тему. TEI-вызов ~200-800ms приемлем. Если TEI unavailable → handler `503`, topic в БД с `embedding = NULL`, следующий MatchJob её пропустит (`rt.embedding IS NOT NULL`).

Error cases:
- Validation → `400 Bad Request`.
- TEI timeout/5xx → `503 Service Unavailable`.

### `POST /radar/feeds`

Request:
```json
{
  "url": "https://news.ycombinator.com/rss",
  "kind": "rss",
  "fetch_interval_seconds": 3600
}
```

Validation:
- `url` — required, http(s), ≤ 2000 символов.
- `kind` — optional, `rss|atom`, default `rss`.
- `fetch_interval_seconds` — optional, 300..86400, default 3600.

Handler flow:
1. Валидация.
2. INSERT feed, без синхронного fetch.
3. `201 Created` + feed object.

Первый crawl через ≤ 5 минут (SchedulerJob tick) или сразу при `RunOnStart: true`.

Error cases:
- Duplicate URL → `409 Conflict`.
- Validation → `400`.

### `POST /radar/subscriptions`

Request:
```json
{ "feed_id": 42 }
```

Handler flow:
1. Валидация.
2. `INSERT ... ON CONFLICT DO NOTHING`.
3. `201 Created` + subscription object. Идемпотентно.

Error cases:
- `feed_id` не существует → FK violation → `404 Not Found`.

### Роутинг в server.go

```go
if cfg.RadarEnabled {
    r.Route("/radar", func(r chi.Router) {
        r.Use(auth.RequireUser)
        r.Post("/topics", radarHTTP.CreateTopic)
        r.Post("/subscriptions", radarHTTP.Subscribe)
        r.Group(func(r chi.Router) {
            r.Use(auth.RequireAdmin)
            r.Post("/feeds", radarHTTP.AddFeed)
        })
    })
} else {
    r.Route("/radar", func(r chi.Router) {
        r.HandleFunc("/*", radarHTTP.DisabledHandler)  // 501 с {"error":"radar_disabled"}
    })
}
```

### OpenAPI

В phase 3a OpenAPI-спеку не пишем. CLI общается с HTTP напрямую. Когда появится frontend — отдельная фаза с `openapi.yaml` и `openapi-typescript` pipeline.

## 7. CLI `linktheca`

### Framework и структура

- `github.com/spf13/cobra` — subcommand routing.
- Путь: `cmd/linktheca/main.go` + `cmd/linktheca/internal/cli/...`.
- Существующий `cmd/linktheca/` переименовывается в `cmd/linktheca-server/` **до** добавления CLI-кода (одним коммитом в начале phase 3a).

### Сессия

```
~/.config/linktheca/session.json       # XDG_CONFIG_HOME/linktheca, permissions 0600
{
  "server_url": "http://localhost:8080",
  "access_token": "eyJ...",
  "refresh_token": "base64string",
  "user_id": 42,
  "is_admin": true,
  "expires_at": "2026-04-22T15:00:00Z"
}
```

Путь переопределяется через `LINKTHECA_CONFIG_DIR` (для тестов).

### Глобальные флаги

```
--server URL          # переопределяет server_url из session.json; env LINKTHECA_URL
--config PATH         # явный путь к session.json
--output FORMAT       # table | json, default table
```

### Subcommands phase 3a

**Auth:**
```
linktheca auth register --email=... --password=... --display-name=...
linktheca auth login --email=... --password=...
linktheca auth logout
linktheca auth whoami
```

`register` и `login` сохраняют токены в session.json. `whoami` → `GET /auth/me` с auto-refresh.

**Radar:**
```
linktheca radar topic add --name="ИИ" --description="..." [--threshold=0.75]
linktheca radar feed add --url="..." [--kind=rss] [--interval=3600]    # admin only
linktheca radar subscribe --feed-id=N
```

Exit code 0 при 2xx, 1 при 4xx/5xx.

### Auto-refresh access token

В HTTP-клиенте CLI:

```go
func (c *APIClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    resp, err := c.httpClient.Do(c.withBearer(req))
    if err != nil { return nil, err }
    if resp.StatusCode == 401 {
        if err := c.refresh(ctx); err != nil {
            return nil, fmt.Errorf("session expired, run `linktheca auth login`: %w", err)
        }
        resp, err = c.httpClient.Do(c.withBearer(req))
    }
    return resp, err
}
```

### Обработка `RADAR_ENABLED=false` на сервере

`linktheca radar *` получая 501 с `{"error":"radar_disabled"}` печатает:

```
Error: Radar is disabled on this server.
Set LINKTHECA_RADAR_ENABLED=true on the backend to enable.
```

### Вне scope phase 3a

- Команды `linktheca library *` (save/list/...) — отдельная фаза.
- CSV/OPML bulk import — отдельная фаза.
- Shell completions (`linktheca completion bash`) — позже.
- TUI/interactive mode — не планируется.

## 8. Тестирование

Четыре уровня + опциональный smoke.

### Level 1: Unit

| Компонент | Файл | Зависимости |
|---|---|---|
| `embeddings.TEIClient` парсинг | `embeddings/client_test.go` | `httptest.Server` |
| `embeddings.FakeEmbedder` | `embeddings/fake_test.go` | stdlib |
| `radar.Service` | `radar/service_test.go` | mock `radar.Store`, `FakeEmbedder` |
| `radar.http` валидация | `radar/http_test.go` | `httptest`, mock Service |
| CLI session | `cmd/linktheca/internal/cli/session/session_test.go` | tmp dir |
| CLI output | `cmd/linktheca/internal/cli/output/format_test.go` | stdlib |
| CLI API client auto-refresh | `cmd/linktheca/internal/cli/apiclient/client_test.go` | `httptest.Server` |

Принцип: `radar.Store` — интерфейс в `service.go`, реализуется реальным `store.go`. Mock'и пишем вручную без gomock/mockery.

### Level 2: Store (реальный Postgres)

Через `internal/testing/testdb`:

| Что | Файл |
|---|---|
| `radar.Store` все методы | `radar/store_test.go` |
| pgvector roundtrip | `radar/store_test.go::TestEmbeddingRoundtrip` |
| HNSW cosine queries | `radar/store_test.go::TestMatchQuery` |
| Crawler с synthetic RSS | `radar/crawler/crawler_test.go` |

Перед phase 3a: проверяю, что `testdb` schema-per-test корректно работает с `vector` типом (extension глобальный, schema его видит).

### Level 3: HTTP integration

| Что | Файл |
|---|---|
| `POST /radar/topics`, `POST /radar/feeds`, `POST /radar/subscriptions` | `radar/integration_test.go` |
| Полный цикл (Crawl → Embed → Match) через реальные jobs | `radar/jobs/jobs_test.go` |

River в тестах: используем либо `river.NewTestClient` / синхронное выполнение workers'ов, либо прямые вызовы `worker.Work(ctx, job)`. Конкретный паттерн — в ходе implementation'а.

### Level 4: Smoke (`-tags=smoke`, не в обычном CI)

| Что | Файл |
|---|---|
| TEI возвращает 1024-dim | `embeddings/client_smoke_test.go` |
| Pipeline с реальным TEI и HN RSS | `radar/jobs/smoke_test.go` |

Запуск:
```
make smoke-radar
```

### Level 5: CLI end-to-end

`cmd/linktheca/integration_test.go`:
1. `go build -o /tmp/linktheca-test ./cmd/linktheca`.
2. Postgres testcontainer + `linktheca-server` в goroutine с `FakeEmbedder`.
3. Последовательный вызов CLI-команд через `exec.Command`.
4. Проверка session.json, SQL-записей, exit codes.

### Тест `RADAR_ENABLED=false`

`server/server_test.go::TestRadarDisabled`:
- Сервер стартует с `RadarEnabled: false`.
- `POST /radar/topics` → 501 с `{"error":"radar_disabled"}`.
- River не содержит radar workers.
- TEI client в DI остался nil.

### Что не тестируем

- Реальный RSS в обычном CI (только synthetic XML).
- Precise whitespace в CLI output (только поля).
- Производительность / benchmarks.
- Chaos tests.

### CI

- `backend` job (существующий) автоматически подхватит новые пакеты.
- Smoke job не добавляем в phase 3a.

## 9. Конфиг, compose, запуск

### Env-переменные phase 3a

```
# TEI
LINKTHECA_TEI_URL=http://tei:8080              # в compose; http://localhost:8081 для dev-локально
LINKTHECA_TEI_TIMEOUT=30s
LINKTHECA_EMBEDDING_DIM=1024

# Radar
LINKTHECA_RADAR_ENABLED=true                   # default true
LINKTHECA_RADAR_SCHEDULER_INTERVAL=5m
LINKTHECA_RADAR_MAX_WORKERS=5

# CLI-only
LINKTHECA_URL=http://localhost:8080
LINKTHECA_CONFIG_DIR=~/.config/linktheca
```

Все существующие переменные phase 1/2 сохраняются.

### Последовательность старта `linktheca-server`

```
1. Parse env → Config
2. Валидация config (JWT secret длина, DSN формат, RADAR_ENABLED парсинг)
3. Подключение Postgres (pgxpool.New)
4. Goose migrations UP (embedded migrations 001-010)
5. River migrations UP
6. Если RADAR_ENABLED:
     - Init TEI client
     - Опциональный self-check Embed("ping"), warning в лог при mismatch
     - Регистрация radar workers (ScheduleCrawls, CrawlFeed, EmbedFinding, MatchFinding)
7. Инициализация River Client, старт
8. HTTP server listen на HTTP_ADDR
```

Shutdown: SIGTERM/SIGINT → HTTP Shutdown → river Stop → pool Close.

### Makefile дополнения

```make
dev-server:           ## run backend with hot reload
	LINKTHECA_TEI_URL=http://localhost:8081 \
	LINKTHECA_DB_DSN=postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable \
	LINKTHECA_JWT_SECRET=dev-only-secret-that-is-at-least-32-bytes-long \
	air

cli-build:            ## build linktheca CLI
	go build -o ./bin/linktheca ./cmd/linktheca

server-build:         ## build linktheca-server
	go build -o ./bin/linktheca-server ./cmd/linktheca-server

smoke-radar:          ## smoke tests with real TEI (slow)
	go test -tags=smoke -timeout=10m -count=1 ./internal/radar/... ./internal/core/embeddings/...
```

### Dev-сценарий после phase 3a

```bash
# Терминал 1: инфраструктура
docker compose -f compose.dev.yaml up -d     # Postgres + TEI

# Терминал 2: backend
make dev-server

# Терминал 3: CLI
./bin/linktheca auth register --email=me@example.com --password=... --display-name="Me"
./bin/linktheca radar feed add --url=https://news.ycombinator.com/rss
./bin/linktheca radar topic add --name="AI" --description="..."
./bin/linktheca radar subscribe --feed-id=1

# Проверка pipeline
psql ... -c "SELECT count(*) FROM radar_findings"
psql ... -c "SELECT * FROM radar_topic_matches ORDER BY matched_at DESC LIMIT 10"
```

### Переименование `cmd/linktheca` → `cmd/linktheca-server`

Одним коммитом в начале phase 3a:
- `git mv cmd/linktheca cmd/linktheca-server`
- Обновить `Dockerfile`, `Makefile`, `.github/workflows/ci.yml`, README, любые `go run ./cmd/linktheca` в docs.
- Package name остаётся `package main`, логика не меняется.

## 10. Риски

| Риск | Mitigation |
|---|---|
| bge-m3 не помещается в память (~2.5-3 GB RSS) | README документирует минимум 4 GB RAM для режима с Radar. Альтернатива `bge-m3-small` (384 dim) — только если подтвердится боль (смена модели = смена миграции) |
| TEI первый старт долгий (скачивание модели) | `start_period: 120s` в healthcheck, `tei-data` volume кеширует между ребутами |
| River periodic jobs дублируются при HA | В phase 3a один инстанс, HA не в scope. River Leader election обеспечит singleton позже |
| RSS feed отдаёт битый XML | gofeed возвращает error, `last_error` сохраняется, River ретраит |
| TEI timeout при большом тексте | `description` ≤ 2000 chars, title+summary из RSS обычно <1000 |
| pgvector/HNSW рост на 100k findings | HNSW масштабируется. Мониторинг размера — phase 3b |
| CLI session.json с secrets на shared машине | Permissions 0600, документируется. OAuth-level security не в scope |
| `RADAR_ENABLED=true`, но TEI недоступен | Handler на `POST /radar/topics` возвращает 503. Topic остаётся без embedding, MatchJob его пропускает. Юзер видит понятное сообщение |

### Принимаемые компромиссы

- Нет дедупликации похожих findings между feeds.
- Нет rate limiting на TEI сверх `MaxWorkers`.
- SchedulerJob ограничен 100 feeds за цикл.

## 11. Опциональный Radar

Один env-флаг — единственный документированный контракт:

```
LINKTHECA_RADAR_ENABLED=true    # default true
```

### Поведение backend'а

**При `true`:**
- TEI client инициализируется, делает self-check Embed.
- River workers Radar регистрируются.
- Роуты `POST /radar/*` монтируются за `RequireUser` / `RequireAdmin`.
- SchedulerJob тикает.

**При `false`:**
- TEI client не создаётся (`embeddings.Client` = nil в DI).
- Radar workers в River не регистрируются.
- Роуты `/radar/*` возвращают `501 Not Implemented` с `{"error":"radar_disabled"}` через catch-all handler на префиксе.
- Миграции 006–010 **выполняются всегда** — таблицы создаются пустыми. Дёшево, упрощает последующее включение без data loss.

### CLI

501 с `radar_disabled` → сообщение:

```
Error: Radar is disabled on this server.
Set LINKTHECA_RADAR_ENABLED=true on the backend to enable.
```

### Требования к ресурсам (для README)

| Конфигурация | RAM минимум | RAM рекомендуется |
|---|---|---|
| `LINKTHECA_RADAR_ENABLED=true` (default) | 4 GB | 8 GB |
| `LINKTHECA_RADAR_ENABLED=false` | 512 MB | 1 GB |

### Что не документируем

Управление TEI-контейнером через compose (scale, удаление, profiles) — оператор решает сам. Документация фокусируется на backend-флаге как единственном контракте фичи.

### Будущие LLM-фичи

Когда появятся (суммаризация, Q&A, автотеги) — отдельные независимые флаги (`LINKTHECA_SUMMARIZATION_ENABLED` и т.п.) с external API ключами. Не завязаны на `RADAR_ENABLED`. Ресурсы локального сервера не меняют.

## Следующие шаги

После approval этого документа — переход к `superpowers:writing-plans` skill для создания пошагового implementation plan'а.
