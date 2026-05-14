# Radar read-API — design

**Дата:** 2026-05-14
**Статус:** approved, готов к writing-plans
**Scope:** backend Radar — read-эндпоинты под предстоящий Radar UI. Без фронтенда.

## Контекст

Crawler, embedding-сервис и matching-worker уже работают (фаза 3a). Backend сейчас умеет создавать темы, подписки и фиды, но `GET`-операций нет. Фронтенд-foundation+auth+library готов; следующий шаг — собрать страницы `/radar` и `/radar/:id`, но для этого UI нужны read-эндпоинты, агрегаты по темам и денормализованные matches.

Этот спек определяет ровно тот минимум backend'a, который нужен SPA для трёх экранов: список тем с метриками, view одной темы с лентой совпадений, admin-список feeds. После него фронтенд-план «Radar UI» будет тонким слоем поверх API.

## Решения, зафиксированные в brainstorming

| # | Вопрос | Решение |
|---|---|---|
| 1 | Что берём в следующий план | Radar backend read-API, без фронта |
| 2 | Основная сущность Radar-ленты | Matches (`radar_topic_matches`), не findings |
| 3 | Pagination style | offset+limit + `total` (как в Library) |
| 4 | Topics list pagination | Не пагинируем — у юзера десятки тем |
| 5 | Topic list форма | Обогащённая `TopicWithStats` с агрегатами (`new_count`, `total_count`, `source_count`, `last_match_at`) |
| 6 | Matches денормализация | Включаем `topic_name`, `feed_title` в `MatchView` |
| 7 | Feeds list (admin) | Включаем в этот план — `GET /radar/feeds` под admin |
| 8 | Owned-by-other-user 404 vs 403 | 404 — не утекаем существование id |
| 9 | Cursor pagination | Откладываем; offset+limit достаточно для MVP |

## 1. Эндпоинты

Все mount под `/radar`, все требуют `RequireUser` (admin-guard где явно отмечено). Ветка `LINKTHECA_RADAR_ENABLED=false` уже отдаёт 501 для всего `/radar/*` через wildcard в `server.go` — новые routes автоматически попадают под него.

| Method | Path | Auth | Назначение |
|---|---|---|---|
| `GET` | `/radar/topics` | user | List тем пользователя с агрегатами. Не пагинируется. |
| `GET` | `/radar/topics/{id}` | user | Одна тема + те же агрегаты. |
| `PATCH` | `/radar/topics/{id}` | user | Обновление полей; `description` пересчитывает embedding. |
| `DELETE` | `/radar/topics/{id}` | user | 204; CASCADE по matches. |
| `GET` | `/radar/matches` | user | `{ items, total }`, query: `topic_id?`, `state?`, `limit`, `offset`. |
| `PATCH` | `/radar/matches/{id}` | user | `{ state }`. |
| `GET` | `/radar/status` | user | `{ last_sweep_at }` — последний `last_fetched_at` по подписанным feeds. |
| `GET` | `/radar/feeds` | **admin** | `{ items, total }`, query: `limit`, `offset`. |

## 2. Формы ответов

Все JSON — snake_case. TS-сторона маппит в camelCase.

### `TopicWithStats`

Возвращается из `GET /radar/topics` (как массив в `{ items }`) и `GET /radar/topics/{id}` (как одиночный объект).

```json
{
  "id": 12,
  "user_id": 1,
  "name": "Local-first software",
  "description": "Apps that put user data on the user's device …",
  "match_threshold": 0.55,
  "is_active": true,
  "has_embedding": true,
  "created_at": "2026-04-22T10:00:00Z",
  "updated_at": "2026-04-25T09:00:00Z",
  "stats": {
    "new_count": 7,
    "total_count": 41,
    "source_count": 5,
    "last_match_at": "2026-05-14T12:01:00Z"
  }
}
```

- `stats.new_count` — `COUNT(*) WHERE state='new'`
- `stats.total_count` — `COUNT(*)` без фильтра
- `stats.source_count` — `COUNT(DISTINCT findings.feed_id)`
- `stats.last_match_at` — `MAX(matched_at)`, nullable

### `MatchView`

Возвращается из `GET /radar/matches` как `items[]`.

```json
{
  "id": 9001,
  "topic_id": 12,
  "topic_name": "Local-first software",
  "similarity": 0.71,
  "state": "new",
  "matched_at": "2026-05-14T12:01:00Z",
  "finding": {
    "id": 4488,
    "feed_id": 3,
    "feed_title": "Hacker News",
    "url": "https://example.com/article",
    "title": "An article",
    "summary": "…",
    "published_at": "2026-05-14T10:00:00Z",
    "discovered_at": "2026-05-14T11:55:00Z"
  }
}
```

`topic_name` и `feed_title` денормализованы намеренно — UI карточки нуждаются в них без extra round-trip. JOIN на topics+feeds дешевле, чем второй query из браузера.

### `Feed`

Уже определён в `internal/radar/types.go`. Форма не меняется. List: `{ items: Feed[], total }`.

### `RadarStatus`

```json
{ "last_sweep_at": "2026-05-14T11:55:00Z" }
```

`last_sweep_at` — `null`, если у юзера нет активных подписок.

### Update DTO

**`UpdateTopicRequest`** (для `PATCH /radar/topics/{id}`):

```json
{
  "name": "…",            // optional
  "description": "…",     // optional, изменение → пересчёт embedding
  "match_threshold": 0.6, // optional, валидация [0, 1]
  "is_active": false      // optional
}
```

Все поля nullable; обновляются только переданные. Минимум одно поле обязательно (иначе `bad_request`).

**`UpdateMatchRequest`**:

```json
{ "state": "seen" }
```

`state` ∈ `{"new", "seen"}`, иначе `bad_request`.

## 3. Ошибки

Используем существующий `writeRadarError`. Дополнений не нужно:

- `400 bad_request` — невалидный JSON, отсутствующее поле в PATCH, плохой enum, threshold вне `[0,1]`, `limit` вне `[1,100]`.
- `404 not_found` — topic/match не найдена или принадлежит другому юзеру.
- `503 embedder_unavailable` — описание темы изменилось, но embedder упал.
- `500 internal` — fallback.

`PATCH` без полей → `bad_request "no fields to update"`.

## 4. Слой store

Расширяем `internal/radar/store.go` следующими методами:

```go
// Topics
ListTopicsWithStats(ctx, userID int64) ([]TopicWithStats, error)
GetTopicWithStats(ctx, userID, topicID int64) (*TopicWithStats, error)
UpdateTopic(ctx, userID, topicID int64, p UpdateTopicParams) (*Topic, error)
DeleteTopic(ctx, userID, topicID int64) error

// Matches
ListMatches(ctx, userID int64, p ListMatchesParams) ([]MatchView, int /*total*/, error)
UpdateMatchState(ctx, userID, matchID int64, state string) error

// Feeds (admin)
ListFeeds(ctx, p ListFeedsParams) ([]Feed, int, error)

// Status
LastSweepAt(ctx, userID int64) (*time.Time, error)
```

`UpdateTopicParams { Name, Description *string; MatchThreshold *float32; IsActive *bool }` — все nullable. Store обновляет только переданные поля; embedding store-методом не трогается. Re-embedding выполняет service отдельным вызовом `UpdateTopicEmbedding` (уже существует), см. §5.

### Ключевые SQL-запросы

**`ListTopicsWithStats`**:

```sql
SELECT
  t.id, t.user_id, t.name, t.description,
  t.match_threshold, t.is_active,
  t.embedding IS NOT NULL AS has_embedding,
  t.created_at, t.updated_at,
  COALESCE(m.new_count, 0)    AS new_count,
  COALESCE(m.total_count, 0)  AS total_count,
  COALESCE(m.source_count, 0) AS source_count,
  m.last_match_at
FROM radar_topics t
LEFT JOIN LATERAL (
  SELECT
    COUNT(*) FILTER (WHERE state = 'new') AS new_count,
    COUNT(*)                              AS total_count,
    COUNT(DISTINCT f.feed_id)             AS source_count,
    MAX(matched_at)                       AS last_match_at
  FROM radar_topic_matches m
  JOIN radar_findings f ON f.id = m.finding_id
  WHERE m.topic_id = t.id
) m ON true
WHERE t.user_id = $1
ORDER BY t.is_active DESC, t.created_at DESC;
```

Существующий индекс `radar_topic_matches_topic_state_idx (topic_id, state, matched_at DESC)` покрывает агрегаты. Дополнительных индексов не добавляем.

**`ListMatches`** — JOIN на topics+feeds, ownership через `t.user_id`:

```sql
SELECT
  m.id, m.topic_id, t.name AS topic_name,
  m.similarity, m.state, m.matched_at,
  f.id AS finding_id, f.feed_id, fd.title AS feed_title,
  f.url, f.title, f.summary,
  f.published_at, f.discovered_at
FROM radar_topic_matches m
JOIN radar_topics t   ON t.id = m.topic_id
JOIN radar_findings f ON f.id = m.finding_id
JOIN radar_feeds fd   ON fd.id = f.feed_id
WHERE t.user_id = $1
  AND ($2::bigint IS NULL OR m.topic_id = $2)
  AND ($3::text   IS NULL OR m.state    = $3)
ORDER BY m.matched_at DESC
LIMIT $4 OFFSET $5;
```

`total` считаем отдельным `COUNT(*)`-запросом с тем же WHERE (паттерн Library).

**`LastSweepAt`**:

```sql
SELECT MAX(f.last_fetched_at)
FROM radar_feeds f
JOIN radar_feed_subscriptions s ON s.feed_id = f.id
WHERE s.user_id = $1 AND f.is_active;
```

**`UpdateMatchState`** — UPDATE с ownership-фильтром:

```sql
UPDATE radar_topic_matches
SET state = $1
WHERE id = $2
  AND topic_id IN (SELECT id FROM radar_topics WHERE user_id = $3)
RETURNING id;
```

Нет затронутых строк → `ErrNotFound`.

**`DeleteTopic`**:

```sql
DELETE FROM radar_topics
WHERE id = $1 AND user_id = $2
RETURNING id;
```

CASCADE снимет matches.

**`UpdateTopic`** — собирается динамически из переданных не-nil полей (паттерн `library.UpdateItem`). Возвращает `(*Topic, embeddingDirty, error)`; service решает, что делать с `embeddingDirty`.

## 5. Слой service

Расширяем `internal/radar/service.go`:

```go
ListTopics(ctx, userID int64) ([]TopicWithStats, error)
GetTopic(ctx, userID, topicID int64) (*TopicWithStats, error)
UpdateTopic(ctx, userID, topicID int64, req UpdateTopicRequest) (*Topic, error)
DeleteTopic(ctx, userID, topicID int64) error

ListMatches(ctx, userID int64, p ListMatchesParams) (*MatchList, error)
SetMatchState(ctx, userID, matchID int64, state string) error

ListFeeds(ctx, p ListFeedsParams) (*FeedList, error)
LastSweep(ctx, userID int64) (*time.Time, error)
```

### Семантика

**`UpdateTopic`:**
- Валидация: те же правила, что в `CreateTopic` для каждого переданного поля — `name` 1..200, `description` 10..2000, `match_threshold` `[0,1]`. Хотя бы одно поле должно быть в patch, иначе `ErrInvalidInput "no fields to update"`.
- Поток (не атомарный, зеркало `CreateTopic`):
  1. `store.UpdateTopic` — пишет переданные поля.
  2. Если `description` присутствовал в patch → `embedder.Embed(name + ": " + description)` → `store.UpdateTopicEmbedding`.
  3. Если embedder упал — возвращаем `ErrEmbedderUnavailable`. Поля уже обновлены, embedding старый. Это согласуется с поведением `CreateTopic` (там тоже остаётся topic без актуального embedding при сбое); фронт может ретраить PATCH с теми же полями. Идемпотентно.
- Решение «embed нужен» принимается по факту присутствия `description` в patch — не сравниваем со старым значением. Лишний пересчёт при `description = "тот же текст"` — редкий случай, не стоит дополнительного round-trip к БД.
- `is_active=false` — matches не удаляем; matching-worker уже фильтрует `WHERE rt.is_active` в `store.MatchFindingToTopics`, поэтому новые findings перестают матчиться к этой теме.

**`SetMatchState`:** валидация enum, прозрачный вызов store.

**`ListMatches`:** валидируем `limit` (1–100, default 50 — паттерн Library), `offset` (≥0, default 0), `state` (enum или nil), `topic_id` (int64 или nil). Чужая тема → пустой результат через `t.user_id` фильтр в WHERE, без отдельной проверки.

**`ListTopics` / `GetTopic`:** тонкие враппинги над store.

**`ListFeeds`:** валидация `limit`/`offset`, прозрачный вызов store. Admin-проверка в middleware на уровне router.

**`LastSweep`:** прозрачный вызов store.

## 6. HTTP-слой

Расширяем `internal/radar/http.go` восемью хендлерами по тому же паттерну, что есть (`XxxHandler() http.HandlerFunc` getter + private handler). Query-params парсим через `r.URL.Query()` (как `library.ListHandler`). Path-params через `chi.URLParam`.

Wiring в `internal/server/server.go` внутри существующего `r.Route("/radar", …)`:

```go
r.Get("/topics", radarHTTP.ListTopicsHandler())
r.Get("/topics/{id}", radarHTTP.GetTopicHandler())
r.Patch("/topics/{id}", radarHTTP.UpdateTopicHandler())
r.Delete("/topics/{id}", radarHTTP.DeleteTopicHandler())

r.Get("/matches", radarHTTP.ListMatchesHandler())
r.Patch("/matches/{id}", radarHTTP.UpdateMatchHandler())

r.Get("/status", radarHTTP.StatusHandler())

r.Group(func(r chi.Router) {
    r.Use(coreauth.RequireAdmin)
    r.Get("/feeds", radarHTTP.ListFeedsHandler())
})
```

Существующий `DisabledHandler` под wildcard `/radar/*` автоматически покрывает новые routes когда `LINKTHECA_RADAR_ENABLED=false`.

## 7. Тестирование

Следуем существующему паттерну (`store_test.go`, `service_test.go`, `http_test.go`, `integration_test.go`). Никаких новых фреймворков. Тесты пишем рядом с существующими.

### Store-тесты (testcontainers + реальный pg+pgvector)

| Тест | Что проверяет |
|---|---|
| `ListTopicsWithStats_empty` | юзер без тем → `[]` |
| `ListTopicsWithStats_aggregates` | две темы с разными counts/sources/state → корректные агрегаты |
| `ListTopicsWithStats_isolation` | темы другого юзера не утекают |
| `GetTopicWithStats_notFound` | чужая тема → `ErrNotFound` |
| `UpdateTopic_partial` | только `name` — `description`/embedding в БД не меняются |
| `UpdateTopic_allFields` | все поля передаются — все обновляются |
| `DeleteTopic_cascades` | удаление снимает связанные matches |
| `ListMatches_filters` | `topic_id`, `state` фильтруют (матрица 4 кейсов) |
| `ListMatches_isolation` | matches чужих тем не возвращаются даже с явным `topic_id` |
| `ListMatches_pagination` | offset/limit + total |
| `ListMatches_ordering` | `matched_at DESC` |
| `UpdateMatchState_ownership` | match чужой темы → `ErrNotFound` |
| `UpdateMatchState_idempotent` | `seen → seen` ок |
| `LastSweepAt_noSubs` | без подписок → `nil` |
| `LastSweepAt_picksMax` | две подписки → max |
| `ListFeeds_pagination` | offset/limit + total |

### Service-тесты (mock store + mock embedder)

| Тест | Что проверяет |
|---|---|
| `UpdateTopic_descriptionTriggersEmbed` | description в patch → store.UpdateTopic вызван, затем embedder, затем store.UpdateTopicEmbedding |
| `UpdateTopic_embedderUnavailable` | embedder упал → `ErrEmbedderUnavailable`; store.UpdateTopic уже вызван (поля обновлены, embedding нет) |
| `UpdateTopic_nameOnly_noEmbed` | только `name` → store.UpdateTopic вызван, embedder НЕ вызван |
| `UpdateTopic_thresholdValidation` | `-0.1`, `1.5` → `ErrInvalidInput` |
| `UpdateTopic_noFields` | пустой patch → `ErrInvalidInput "no fields to update"` |
| `SetMatchState_validation` | `"foo"` → `ErrInvalidInput` |
| `ListMatches_clampLimit` | `limit=200` → 100; `limit=0` → 50 (default) |

### HTTP-тесты (httptest + mock service)

Для каждого нового handler: happy path, `not_found`, `bad_request`, авторизация. Отдельный тест на admin-guard для `/feeds` (non-admin user → 403). Декодинг query-params (`limit`, `offset`, `state`, `topic_id`) проверяем явно.

### Integration-тест (один сценарий, расширяет `integration_test.go`)

1. Регистрация юзера, создание темы, подписка на feed.
2. Прямой `INSERT` finding + match (минуя crawler).
3. `GET /radar/topics` → проверяем `stats` агрегаты.
4. `GET /radar/matches?state=new` → один item с денормализованными `topic_name`/`feed_title`.
5. `PATCH /radar/matches/{id}` `{state:"seen"}`.
6. `GET /radar/topics` → `new_count` уменьшился.
7. `DELETE /radar/topics/{id}` → 204.
8. `GET /radar/matches` → пустой (CASCADE сработал).

### Что НЕ тестируем

- Performance/load.
- HNSW recall (покрыто в pipeline-плане).
- Сам embedder (мокаем).
- Wiring `LINKTHECA_RADAR_ENABLED=false` — уже покрыто существующими тестами через wildcard.

## 8. Edge cases

- **Topic без embedding** (embedder упал при `CreateTopic` или предыдущем `UpdateTopic`) — `has_embedding=false` в ответе. Фронт может повторить PATCH с `description`, чтобы инициировать пересчёт. Embed синхронный, не через job-queue.
- **Match чужой темы** — 404, не 403. Аналогично для DELETE/PATCH topic.
- **Конкурентный PATCH** — last-writer-wins, оптимистичная блокировка не нужна.
- **DELETE темы во время PATCH с re-embed** — теоретически возможен race: одновременный DELETE и PATCH-description. Если PATCH успел перед DELETE → DELETE снимает всё через CASCADE. Если PATCH между UPDATE-полей и `UpdateTopicEmbedding` → `UpdateTopicEmbedding` вернёт `ErrNotFound`, service вернёт 404. Приемлемо.
- **`limit=0`** — service подставляет default 50 (паттерн Library).
- **Match-worker уже фильтрует `is_active`** — проверено в `store.MatchFindingToTopics` (line 243 `AND rt.is_active`). Bug-fix не нужен.

## 9. Что явно вне scope

- Frontend (Radar UI) — следующий план.
- Cursor-pagination.
- `GET /radar/findings` — UI не показывает сырые findings.
- Per-topic threshold slider live preview — отложено (`project_radar_threshold_slider_deferred.md`).
- User-added feeds, отписка от feed — отложено (`project_user_added_feeds_deferred.md`).
- Удаление/деактивация feeds (admin).
- Поиск по matches/findings.
- Search по темам.
- Уведомления о новых matches.

## Следующие шаги

После approval этого документа — переход к `superpowers:writing-plans` для одного плана:

- `2026-05-XX-radar-read-api.md` — store/service/http методы, миграций не нужно, тесты, расширение `server.go`.

После него отдельным планом пойдёт фронтенд `radar-ui.md` (страницы `/radar` и `/radar/:id`, AddTopicDialog, edit/delete, отметка matches как seen).