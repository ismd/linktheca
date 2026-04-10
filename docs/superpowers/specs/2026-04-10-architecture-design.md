# Linktheca: архитектурный дизайн MVP

**Дата:** 2026-04-10
**Статус:** approved, готов к writing-plans

## Контекст

Linktheca — open source self-hosted read-it-later сервис с мониторингом новостей по темам через семантический поиск на локальных embeddings.

Модули:
- **Library** (read-it-later) — ручное сохранение ссылок пользователем.
- **Radar** (мониторинг новостей) — автоматическая находка релевантных статей по подписанным темам.
- **Core** — общие сервисы: auth, парсинг контента, embeddings, БД.

**Ключевое UX-решение (из CLAUDE.md):** Library и Radar не смешиваются. Пользовательские сохранения и автоматические находки живут в разных разделах.

## Констрейнты и решения верхнего уровня

Приняты в процессе brainstorming:

| # | Вопрос | Решение |
|---|---|---|
| 1 | Модель пользователей | Multi-user self-hosted, регистрация закрывается через env |
| 2 | Масштаб одного инстанса | Средний: ~100 юзеров, сотни тысяч статей. В перспективе — shared public instance (но не оптимизируем под это сейчас) |
| 3 | Scope MVP | Library полноценно + минимальный Radar (RSS only, embeddings, без уведомлений) |
| 4 | Mobile | First-class с первого дня: чистое JSON API, Bearer tokens |
| 5 | Деплой | Docker Compose |
| 6 | Опыт разработчика | Новичок в Go — предпочитаем идиоматичные и простые решения |

## Архитектура верхнего уровня

Монолит с чистыми границами между модулями. Один Go-модуль, один бинарь, границы между `library`/`radar`/`core` обеспечиваются через `internal/` пакеты и направление импортов.

### Docker Compose shape

```
docker compose
├── web          nginx со статикой собранного React
├── backend      Go бинарь: HTTP API + workers (River) в одном процессе
├── postgres     Postgres 16 + pgvector (данные, векторы, River queue, FTS)
└── ollama       embedding-сервер, bge-m3 модель
```

Mobile-клиент живёт в **отдельном репозитории** (`linktheca-mobile`), это данный репо не затрагивает.

## 1. Репо-layout

```
linktheca/
├── CLAUDE.md
├── README.md
├── docker-compose.yml
├── docker-compose.dev.yml
├── docker-compose.prod.yml
├── Makefile
│
├── cmd/
│   └── linktheca/main.go           # единственный бинарь backend
│
├── internal/
│   ├── core/
│   │   ├── config/                 # загрузка конфига через env
│   │   ├── db/                     # pgx pool, миграции через embed
│   │   ├── httpx/                  # middleware, response helpers
│   │   ├── auth/                   # JWT, argon2id, middleware
│   │   ├── content/                # парсинг статей (readability)
│   │   ├── embeddings/             # клиент к Ollama
│   │   └── logging/                # slog
│   ├── library/                    # read-it-later модуль
│   │   ├── types.go
│   │   ├── store.go                # SQL через pgx
│   │   ├── service.go              # бизнес-логика
│   │   └── http.go                 # HTTP handlers
│   ├── radar/                      # мониторинг новостей
│   │   ├── types.go
│   │   ├── store.go
│   │   ├── service.go
│   │   ├── http.go
│   │   ├── crawler/                # RSS/Atom fetcher (gofeed)
│   │   └── jobs/                   # River workers
│   └── server/
│       └── server.go               # DI, собирает chi router
│
├── migrations/                     # SQL миграции (goose)
│
├── web/                            # React SPA — отдельный npm-проект
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.ts
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── routes/
│       ├── features/
│       │   ├── auth/
│       │   ├── library/
│       │   └── radar/
│       ├── shared/
│       │   ├── api/
│       │   ├── ui/                 # shadcn/ui компоненты
│       │   ├── layout/
│       │   ├── hooks/
│       │   └── lib/
│       └── styles/
│
├── prototype/index.html            # существующий визуальный прототип, не трогаем
│
├── docs/superpowers/specs/         # spec-документы
│
├── go.mod                          # один Go-модуль на весь backend
├── go.sum
└── .github/workflows/              # CI
```

**Ключевые решения:**

- **Один `go.mod` на весь backend.** Не workspace, не multi-module. Границы между модулями обеспечиваются через `internal/` и направление импортов.
- **Модули `library` и `radar` не импортят друг друга напрямую.** Общение — только через интерфейсы, объявленные в месте использования (см. раздел «Общение между модулями»).
- **Один бинарь (`cmd/linktheca`).** API и workers в одном процессе. При необходимости разделения — добавим второй `cmd/worker` позже.
- **`web/` — отдельный npm-проект в том же репо.** Монорепо из-за синхронных изменений API↔UI, общего релизного цикла, одного CI.
- **Frontend билдится в nginx-образ** на этапе Docker build, в compose это отдельный контейнер `web`.

## 2. Backend: библиотеки и внутреннее устройство

### Библиотеки

| Задача | Библиотека | Роль |
|---|---|---|
| HTTP роутинг | `go-chi/chi/v5` | Тонкий слой над `net/http` |
| Postgres driver | `jackc/pgx/v5` | Pool, типы, нативная поддержка pgvector |
| Векторы в pgx | `pgvector/pgvector-go` | `vector` type для pgx |
| SQL | raw SQL через pgx | Никаких ORM, никакой кодогенерации на MVP. Если SQL станет многословным — введём `sqlc` позже отдельным решением |
| Миграции | `pressly/goose` | `.sql` файлы, запуск из embed при старте |
| Job queue | `riverqueue/river` | Postgres-backed, для crawler/embed/match |
| Валидация | `go-playground/validator/v10` | Теги на структурах |
| JWT | `golang-jwt/jwt/v5` | Access-токены |
| Password hashing | `golang.org/x/crypto/argon2` | argon2id с PHC-строкой |
| Парсинг контента | `go-shiori/go-readability` | Извлечение текста статей |
| RSS/Atom | `mmcdole/gofeed` | Парсер feeds |
| Логи | `log/slog` (stdlib) | Structured logging |
| Конфиг | `caarlos0/env` | Env vars → struct |
| Rate limit | `go-chi/httprate` | In-memory token bucket для /login, /register |
| CORS | `go-chi/cors` | CORS middleware |
| Тесты: контейнеры | `testcontainers-go` | Реальный Postgres для integration-тестов |
| Тесты: assertions | `stretchr/testify` | `assert`, `require` |
| Auto-reload (dev) | `air-verse/air` | Пересборка бинаря на изменения |

**Намеренно не используем:** GORM/ent (ORM скрывает SQL), Gin/Echo/Fiber (больше магии), viper (env достаточно), Redis/asynq (River покрывает через Postgres).

### Паттерн каждого модуля: `store → service → http`

- **`store`** — знает только про БД. Методы возвращают доменные типы. Никакой HTTP, никакой бизнес-логики.
- **`service`** — бизнес-логика. Комбинирует store и внешние зависимости (`content.Extractor`, `embeddings.Client`). Не знает про HTTP. Принимает store через интерфейс (для тестов).
- **`http`** — тонкие handlers. Парсят запрос, вызывают service, сериализуют ответ. Никакой логики.

### Общение между модулями

Принцип: **`library` и `radar` никогда не импортят друг друга напрямую.**

Единственная точка пересечения на MVP — «переместить находку из Radar в Library». Решение: `radar` объявляет интерфейс того, что ему нужно, `server.go` при сборке передаёт реализацию из `library`:

```go
// в internal/radar
type LibrarySaver interface {
    Save(ctx context.Context, userID int64, url string) error
}
```

Стандартный Go-паттерн: интерфейсы объявляются там, где используются.

## 3. Модель данных (Postgres + pgvector)

Extension `pgvector` создаётся в первой миграции: `CREATE EXTENSION IF NOT EXISTS vector;`

### Блок core

```sql
-- Пользователи системы
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,          -- argon2id PHC string
    display_name  TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Refresh-токены (access — stateless JWT, refresh — серверный, отзываемый)
CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,      -- хэш refresh-токена, оригинал не храним
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON refresh_tokens (user_id) WHERE revoked_at IS NULL;

-- Общий кэш распарсенного контента: один URL парсится максимум раз,
-- потом на него ссылаются и library, и radar без дублирования текста.
CREATE TABLE article_contents (
    id                   BIGSERIAL PRIMARY KEY,
    url                  TEXT NOT NULL UNIQUE,
    canonical_url        TEXT,
    title                TEXT,
    byline               TEXT,
    excerpt              TEXT,
    text                 TEXT,            -- plain text
    html                 TEXT,            -- очищенный HTML для reader view
    lang                 TEXT,
    reading_time_seconds INT,
    fetched_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    fetch_error          TEXT
);
CREATE INDEX ON article_contents USING GIN (
    to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(text,''))
);
```

**Примечание:** регистрация управляется env-переменной `LINKTHECA_REGISTRATION_ENABLED`, отдельной таблицы настроек нет.

### Блок library

```sql
CREATE TABLE library_items (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id  BIGINT NOT NULL REFERENCES article_contents(id),
    state       TEXT NOT NULL DEFAULT 'unread'
                CHECK (state IN ('unread', 'read', 'archived')),
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    note        TEXT,
    saved_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ,
    UNIQUE (user_id, content_id)
);
CREATE INDEX ON library_items (user_id, saved_at DESC);
CREATE INDEX ON library_items (user_id, state);
```

**Тегов в MVP нет.** Добавим позже отдельной миграцией.

### Блок radar

```sql
-- Темы, на которые подписан юзер
CREATE TABLE radar_topics (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,           -- текст для embedding
    embedding       vector(1024),            -- bge-m3 размерность
    match_threshold REAL NOT NULL DEFAULT 0.75,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON radar_topics (user_id) WHERE is_active;

-- Feeds глобальные: один источник парсится один раз для всех подписанных юзеров
CREATE TABLE radar_feeds (
    id                     BIGSERIAL PRIMARY KEY,
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
CREATE INDEX ON radar_feeds (is_active, last_fetched_at);

-- Подписки: даже если в MVP UI не будет — таблицу заводим сразу
CREATE TABLE radar_feed_subscriptions (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id    BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feed_id)
);

-- Сырые находки до семантической фильтрации
CREATE TABLE radar_findings (
    id            BIGSERIAL PRIMARY KEY,
    feed_id       BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    content_id    BIGINT REFERENCES article_contents(id),  -- NULL пока не открыли
    external_id   TEXT,                                    -- id/guid из RSS
    url           TEXT NOT NULL,
    title         TEXT,
    summary       TEXT,
    embedding     vector(1024),                            -- bge-m3
    published_at  TIMESTAMPTZ,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (feed_id, external_id)
);
CREATE INDEX ON radar_findings (discovered_at DESC);
CREATE INDEX ON radar_findings USING hnsw (embedding vector_cosine_ops);

-- Матчинг: это то, что юзер видит в Radar UI
CREATE TABLE radar_topic_matches (
    id         BIGSERIAL PRIMARY KEY,
    topic_id   BIGINT NOT NULL REFERENCES radar_topics(id) ON DELETE CASCADE,
    finding_id BIGINT NOT NULL REFERENCES radar_findings(id) ON DELETE CASCADE,
    similarity REAL NOT NULL,
    state      TEXT NOT NULL DEFAULT 'new'
               CHECK (state IN ('new', 'seen')),
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic_id, finding_id)
);
CREATE INDEX ON radar_topic_matches (topic_id, state, matched_at DESC);
```

**Ключевые свойства схемы:**

- **`article_contents` лежит в core** и шарится между library и radar. Один URL парсится один раз.
- **`radar_findings` глобальные**, `radar_topic_matches` per-user. Один feed парсится один раз для всех подписанных юзеров.
- **«Сохранено в Library»** определяется JOIN'ом `radar_findings` → `article_contents` ← `library_items`, без отдельного state-поля.
- **vector(1024)** для `bge-m3` модели (выбор обоснован в разделе «Embeddings»).
- **HNSW index** на `radar_findings.embedding` — быстрый kNN поиск с cosine distance.
- **Миграции запускаются автоматически при старте backend'а** (embedded через `//go:embed migrations/*.sql`).

## 4. Auth flow

### Принцип

Один механизм для web и mobile. Вся авторизация через `Authorization: Bearer <access>` header. Никаких cookies.

### Модель токенов

| Токен | Тип | Время жизни | Где хранится |
|---|---|---|---|
| Access | JWT HS256 (stateless) | 15 минут | Клиент: память. Сервер: нигде |
| Refresh | Случайная строка (32 байта base64) | 30 дней | Клиент: secure storage. Сервер: хэш в `refresh_tokens` |

**Обоснования:**
- Access — JWT, чтобы валидировать каждый запрос без удара по БД.
- Refresh — opaque токен в БД, чтобы можно было отозвать. JWT отозвать нельзя.
- Короткий access минимизирует ущерб от утечки; refresh rotation обнаруживает повторное использование.

### Claims access JWT

```json
{
  "sub": "42",
  "iat": 1712750000,
  "exp": 1712750900,
  "is_admin": false
}
```

Claims минимальны — только то, что нужно middleware. Email, display_name и т.п. достаются из `/auth/me`.

### Endpoints

```
POST /auth/register  { email, password, display_name }
POST /auth/login     { email, password }
POST /auth/refresh   { refresh_token }                   (rotation: старый → revoked)
POST /auth/logout    + Bearer  { refresh_token }
GET  /auth/me        + Bearer
```

- `/register` возвращает 403, если `LINKTHECA_REGISTRATION_ENABLED=false`.
- `/register` и `/login` rate-limited через `httprate` (10 попыток на IP / 10 минут).
- `/refresh` обязательно делает rotation: выдаёт новый refresh, помечает старый `revoked_at`.

### Middleware

- `auth.RequireUser` — парсит Bearer, валидирует JWT, кладёт `userID` и `is_admin` в context.
- `auth.RequireAdmin` — обязательно поверх `RequireUser`, 403 если `!is_admin`.

Группы роутов в `server.go`:
- `/auth/*` — публичные + защищённые внутри
- `/library/*`, `/radar/*` — все за `RequireUser`
- `/admin/*` — за `RequireUser + RequireAdmin`

### Первый пользователь = админ

При регистрации: если в БД ещё нет юзеров, новый юзер создаётся с `is_admin=true`. Никаких seed-скриптов, никакого setup-wizard.

### Безопасность

| Что | Как |
|---|---|
| Хэш паролей | argon2id, параметры: `time=2, memory=64MB, threads=2, keyLen=32, salt=16 байт`. Формат хранения — стандартный PHC string |
| Минимальная длина пароля | 10 символов |
| Rate limit | `httprate` in-memory, 10/10min на `/login` и `/register` |
| HTTPS | Ответственность оператора (Caddy/Traefik снаружи compose) |
| CORS | `LINKTHECA_CORS_ORIGINS` env, пустой по умолчанию |

**Не входит в MVP:** email verification, password reset, 2FA, OAuth, audit log, admin UI для управления юзерами.

## 5. Radar pipeline

Четыре типа работы, каждый — отдельный River job:

```
Scheduler → CrawlFeedJob → EmbedJob → MatchJob
```

### Scheduler (periodic River job)

Каждые 5 минут:
```sql
SELECT * FROM radar_feeds
WHERE is_active
  AND (last_fetched_at IS NULL
       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
LIMIT 100
```
Для каждого — `river.Insert(CrawlFeedJob{FeedID: id})`.

### CrawlFeedJob

1. HTTP GET feed с `If-None-Match` (etag) и `If-Modified-Since`. Если 304 — только обновить `last_fetched_at`.
2. Распарсить через `gofeed`.
3. Для каждого item: `INSERT INTO radar_findings (content_id=NULL, embedding=NULL) ON CONFLICT (feed_id, external_id) DO NOTHING`.
4. Для каждой новой вставки: `river.Insert(EmbedJob{FindingID: id})`.
5. `UPDATE radar_feeds SET last_fetched_at, etag, last_modified, last_error=NULL`.

**Важно:** в этом шаге НЕ парсим полный контент. Только то, что даёт сам feed — title, summary, published_at, url.

River retry: exponential backoff до 25 попыток за ~24 часа. При failed job — `last_error` сохраняется в `radar_feeds`.

### EmbedJob

1. Достать finding. Если `embedding IS NOT NULL` — выйти (идемпотентность).
2. `text := title + "\n" + summary`.
3. `vec := ollama.Embed(text)` — HTTP к Ollama, модель `bge-m3`, 1024 dim.
4. `UPDATE radar_findings SET embedding=$vec`.
5. `river.Insert(MatchJob{FindingID: id})`.

**Эмбеддим title+summary, не полный текст**. Экономия + для короткой новости достаточно. Fetch полного контента — по требованию (см. шаг 6).

### MatchJob

Один SQL на всё:
```sql
INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
SELECT rt.id, $1, 1 - (rt.embedding <=> $2), 'new'
FROM radar_topics rt
JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
WHERE rfs.feed_id = $3
  AND rt.is_active
  AND 1 - (rt.embedding <=> $2) >= rt.match_threshold
ON CONFLICT (topic_id, finding_id) DO NOTHING;
```

`<=>` — cosine distance от pgvector. `match_threshold` юзер задаёт в UI (диапазон 0..1).

### Отображение в UI

`GET /radar/feed`:
```sql
SELECT f.*, m.similarity, m.state, t.name AS topic_name,
       EXISTS(SELECT 1 FROM library_items li
              WHERE li.user_id = $userID AND li.content_id = f.content_id)
         AS in_library
FROM radar_topic_matches m
JOIN radar_findings f ON f.id = m.finding_id
JOIN radar_topics t ON t.id = m.topic_id
WHERE t.user_id = $userID AND m.state = 'new'
ORDER BY m.matched_at DESC
LIMIT 50;
```

### Fetch full content — по требованию

`GET /radar/findings/:id/read`:
1. Если `finding.content_id IS NULL` — `content.Extract(url)` → INSERT в `article_contents` → UPDATE finding.
2. Вернуть контент.

Тот же механизм для Library: при сохранении URL `content.Extract` вызывается синхронно в handler'е. Если тот же URL уже есть в `article_contents` — переиспользуем.

### Edge cases

- **Изменение embedding темы** (юзер отредактировал description): перегенерируем `radar_topics.embedding`. Существующие matches не пересчитываем — они «исторические». Новые находки матчатся против нового embedding.
- **Feed ломается**: River ретраит, `last_error` сохраняется, `last_fetched_at` всё равно обновляется (чтобы не долбиться). Админ видит сломанные feeds в UI (phase 2).

### Вне MVP

Уведомления (любые), HTML-скрапинг non-RSS источников, авто-обнаружение feeds на сайтах, reranking/cross-encoder, дедупликация похожих находок, fine-tuning threshold по feedback'у юзера.

## 6. Embeddings

**Сервер:** Ollama — как отдельный сервис в compose, HTTP API.

**Обоснование Ollama (не TEI, не in-process):**
- В будущем точно понадобятся LLM-фичи (автосуммаризация статей, помощь с описанием тем).
- Ollama покрывает и embeddings, и LLM одним сервисом — добавить новую модель это `ollama pull`.
- TEI был бы точнее под embeddings-only, но не масштабируется на LLM.
- fastembed-go экономит контейнер, но тащит C-зависимости в Go-бинарь.

**Модель:** `bge-m3`, 1024 dim, 2.3 GB на диске.

**Обоснование bge-m3:** мультиязычная, хорошо работает на русском и английском одновременно. Размер диска в self-hosted не ограничивает.

**Критичное ограничение:** размерность embedding'а нельзя поменять без пересчёта всех векторов и пересоздания HNSW индексов. Выбор модели — долгосрочный.

**Go-клиент:** простой HTTP-вызов, `POST http://ollama:11434/api/embeddings { model: "bge-m3", prompt: text }`, распарсить массив `float32`, завернуть в `pgvector.Vector`. Интерфейс `embeddings.Client.Embed(ctx, text) ([]float32, error)` — для тестов подменяется детерминированным моком.

## 7. Frontend

### Стек

| Задача | Выбор |
|---|---|
| Build | Vite |
| Язык | TypeScript (strict) |
| UI framework | React 19 |
| Роутинг | React Router v7 (data mode) |
| Server state | TanStack Query v5 |
| Client state | Zustand |
| Стили | Tailwind CSS v4 |
| Компоненты | shadcn/ui + Radix primitives (копируются в проект) |
| Шрифты | Fraunces + Newsreader + JetBrains Mono (из прототипа) через @fontsource |
| Формы | React Hook Form + Zod |
| Иконки | lucide-react |
| Тесты: unit | Vitest + React Testing Library + MSW |
| Тесты: e2e | Playwright (phase 2, после MVP) |

**Не используем:** Next.js/Remix (нужен SPA), Redux (Zustand достаточно), MUI/Chakra (диктуют дизайн, перебьют editorial-эстетику прототипа), Emotion/styled-components (Tailwind).

### Структура `web/src/`

```
main.tsx, App.tsx

routes/                             # React Router tree
    index, login, register
    library/ (index, $id)
    radar/ (index, topics, findings.$id)
    settings/

features/                           # feature-based, зеркалит backend
    auth/     (api, hooks, store, components)
    library/  (api, hooks, components)
    radar/    (api, hooks, components)

shared/
    api/      (client, errors, types — типы генерируются из OpenAPI)
    ui/       (shadcn/ui компоненты)
    layout/   (AppShell, ReaderLayout)
    hooks/
    lib/

styles/globals.css
```

Принцип — **feature-based, не type-based**. Всё связанное с Library лежит в `features/library`.

### API клиент

- **OpenAPI-спека поддерживается руками** (`openapi.yaml` в корне или в `web/`).
- `openapi-typescript` генерирует `web/src/shared/api/types.ts` как dev-скрипт `npm run gen:api`.
- Тонкий fetch wrapper в `shared/api/client.ts`: добавляет Bearer, обрабатывает 401 (автоматический refresh + retry, при неуспехе — logout).
- Функции в `features/*/api.ts` — обёртки над `apiFetch` с типами из сгенерированного `types.ts`.
- TanStack Query хуки в `features/*/hooks.ts` поверх API-функций.

### Dev сценарий

Три терминала:
1. `docker compose -f docker-compose.dev.yml up` — Postgres + Ollama.
2. `air` или `make dev-backend` — backend с auto-reload.
3. `cd web && npm run dev` — Vite dev server с HMR.

Vite проксирует `/api/*` на `localhost:8080`, CORS не нужен.

### Deployment

Отдельный контейнер `web`:
- Multi-stage Dockerfile: `node:22` для билда → `nginx:alpine` для раздачи.
- Nginx раздаёт `dist/` и проксирует `/api/*` на `backend:8080`.

## 8. Тестирование

### Backend

| Уровень | Инструменты | Покрытие |
|---|---|---|
| Unit | `testing` + `testify/assert` | `service.go` с mock-store через интерфейсы |
| Integration (store) | `testcontainers-go` + `pgx` | SQL против реального Postgres с pgvector |
| HTTP | `httptest` + testcontainers | Полный путь HTTP → service → store → БД |

**Принцип: никаких моков БД.** pgvector, HNSW, `<=>`, FTS — Postgres-специфика, мокать нельзя.

**Паттерн `testdb.New(t)`:**
- Один раз на test run — `sync.Once` поднимает Postgres testcontainer.
- На каждый `t.Run` — `CREATE SCHEMA test_xyz`, мигрирует, возвращает pool с `search_path`.
- `t.Cleanup` дропает схему.

**Ollama в тестах:** не вызываем реальный сервис. `embeddings.Client` — интерфейс, в тестах подменяется детерминированным моком (например, hash → вектор). Smoke test с реальной Ollama — под build tag `-tags=smoke`, вне основного прогона.

### Frontend

| Уровень | Инструменты |
|---|---|
| Component | Vitest + React Testing Library + MSW |
| E2E (phase 2) | Playwright |

Компонентные тесты покрывают: формы (валидация, submit, ошибки), хуки (loading/error/success), сложные компоненты со state.

**Не тестируем:** visual regression, storybook, каждую кнопку.

E2E критичных путей откладываем на phase 2 после MVP.

### CI

GitHub Actions:
- `backend`: `go vet` + `go test ./... -race`
- `frontend`: lint + type-check + vitest + build
- `e2e`: после backend+frontend, только на PR в main (phase 2)

## 9. Dev-опыт

### Makefile

```
make dev          # compose dev + air + vite (всё в одном)
make test         # go test + npm test
make test-e2e     # playwright
make migrate      # goose up
make lint         # go vet + npm lint
make build        # полный прод-билд
```

### Конфиг

Через env vars, парсятся `caarlos0/env` в структуру `Config`. Полный список MVP-настроек:

```
# Сервер
LINKTHECA_HTTP_ADDR=:8080
LINKTHECA_LOG_LEVEL=info              # debug | info | warn | error
LINKTHECA_LOG_FORMAT=text              # text | json

# База данных
LINKTHECA_DB_DSN=postgres://linktheca:linktheca@postgres:5432/linktheca?sslmode=disable

# Auth
LINKTHECA_JWT_SECRET=<32+ bytes random>
LINKTHECA_JWT_ACCESS_TTL=15m
LINKTHECA_JWT_REFRESH_TTL=720h         # 30 дней
LINKTHECA_REGISTRATION_ENABLED=true    # управляет POST /auth/register

# CORS (для dev — пустой, т.к. Vite проксирует через /api)
LINKTHECA_CORS_ORIGINS=

# Ollama / embeddings
LINKTHECA_OLLAMA_URL=http://ollama:11434
LINKTHECA_EMBEDDING_MODEL=bge-m3

# Radar
LINKTHECA_RADAR_SCHEDULER_INTERVAL=5m  # как часто сканируем radar_feeds
```

Файлы:
```
.env                  # shared defaults, коммитим (не секреты)
.env.local            # gitignore, локальные секреты
docker-compose.yml    # base
docker-compose.dev.yml
docker-compose.prod.yml
```

### Наблюдаемость в MVP

Только **structured logs** через `log/slog`:
- Формат: JSON в проде, text в dev.
- Request ID из middleware прокидывается во все log-линии запроса.

**Не в MVP:** Prometheus metrics, OpenTelemetry tracing, Sentry, dashboards.

## Что явно вне MVP

Сведено из всех секций:

- Теги в Library.
- State `reading` в Library (только `unread`, `read`, `archived`).
- Состояния `dismissed`, `saved` в Radar matches (только `new`, `seen`).
- Email verification, password reset, 2FA, OAuth/SSO, audit log, admin UI.
- Уведомления (email/push/in-UI).
- HTML-скрапинг non-RSS источников.
- Авто-обнаружение feeds на URL сайта.
- Reranking/cross-encoder в Radar.
- Дедупликация похожих findings.
- LLM-фичи (суммаризация и т.п.) — Ollama готова к ним, но сами фичи не реализуем.
- E2E тесты (Playwright).
- Метрики, tracing, dashboards.
- Mobile клиент (отдельный репозиторий, отдельный проект).

## Следующие шаги

После approval этого документа — переход к `superpowers:writing-plans` skill для создания пошагового implementation plan'а.