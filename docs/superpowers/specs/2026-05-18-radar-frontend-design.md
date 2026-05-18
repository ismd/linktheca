# Radar frontend (Topic UI) — design

**Дата:** 2026-05-18
**Статус:** approved, готов к writing-plans
**Scope:** Radar UI поверх готового read-API + точечный backend-довесок `GET /radar/matches/{id}`. Полный CRUD топиков, чтение матчей, match reader, mark-seen, отключение «Radar disabled» state.

## Контекст

Backend Radar read-API завершён (spec `2026-05-14-radar-read-api-design.md`). Frontend foundation, auth и Library реализованы; sidebar содержит `Radar (disabled)`-пункт. Прототип в `prototype/index.html` определяет визуальный язык Radar и Topic view.

В этой фазе превращаем заглушенный пункт меню в живой раздел Radar.

## Решения, зафиксированные в brainstorming

| # | Вопрос | Решение |
|---|---|---|
| 1 | Scope | Полный CRUD топиков + чтение матчей + match reader. Bulk «mark all seen» отложено. |
| 2 | Keywords (chips из прототипа) | Не выводим. Embedding считается из `name + description`; добавление `topics.keywords` отложено в [[project-radar-sources-not-per-topic]]. |
| 3 | Sources (блок «Sources being watched» из прототипа) | Не выводим. Subscriptions — per-user (миграция `008_radar_feed_subscriptions`), не per-topic. Конкретный список «sources for this topic» не определён в модели. `source_count` остаётся как denormalized stat (число, не список). |
| 4 | Match click → куда | Внутренний reader `/radar/matches/$matchId`, визуально как library reader; body = `summary`; крупная CTA «Open original»; action «Save to library». |
| 5 | Match reader data fetching | Approach A: добавить backend `GET /radar/matches/{id}` для direct-URL и refresh support. Альтернативы (передача через router state, объединение Topic view + reader) отклонены. |
| 6 | Mark seen lifecycle | Auto on reader open (mount-эффект → PATCH /radar/matches/{id} state="seen"). |
| 7 | Embedder 503 при Create/Update | Specific toast «Embedder unavailable, retry later». Generic-toast только для остальных ошибок. Modal/Dialog не закрывается при 503. |
| 8 | Threshold slider | Не показываем в Create/Edit (`project-radar-threshold-slider-deferred`). Backend использует default. |
| 9 | Subscription management | Не в UI; admin/CLI добавляет feeds, юзер подписывается через API (`project-user-added-feeds-deferred`). |
| 10 | Full-content reader для match'а | Нет; summary only (`project-radar-content-extraction-deferred`). |

## 1. Архитектура

Зеркалит структуру `features/library/` плюс одно расширение backend'а.

### Frontend

```
web/src/features/radar/
  api.ts                  fetch-функции, тонкая обёртка над apiFetch
  schemas.ts              Zod-схемы для raw API + map-функции snake_case → camelCase
  types.ts                TopicWithStats, MatchView, RadarStatus, FilterParams, PAGE_SIZE
  use-radar.tsx           useTopicsQuery, useTopicQuery, useMatchesQuery (infinite),
                          useMatchQuery, useRadarStatusQuery
  use-mutations.tsx       useCreateTopic, useUpdateTopic, useDeleteTopic, useMarkMatchSeen
  components/
    TopicCard.tsx
    TopicGrid.tsx
    MatchCard.tsx
    MatchGrid.tsx
    NewTopicDialog.tsx
    EditTopicDialog.tsx
    DeleteTopicConfirm.tsx
    TopicHeader.tsx
    StatsLine.tsx
    SkeletonCard.tsx      (если library-вариант не получится переиспользовать as-is)
    EmptyTopicList.tsx
    EmptyTopicMatches.tsx

web/src/routes/
  radar._index.tsx        /radar
  radar.$topicId.tsx      /radar/$topicId
  radar.matches.$matchId.tsx  /radar/matches/$matchId

web/src/shared/layout/Sidebar.tsx
  снять disabled: true с Radar nav-пункта
```

### Backend addition (`internal/radar/`)

Точечное расширение, симметричное `GetTopic`.

| Слой | Метод/Handler | Изменение |
|---|---|---|
| Store | `GetMatch(ctx, userID, matchID) (*MatchView, error)` | Новый. SQL — тот же JOIN, что в `ListMatches`, плюс `WHERE m.id = $1 AND t.user_id = $2`. Чужой match → `ErrMatchNotFound`. |
| Service | `GetMatch(ctx, userID, matchID) (*MatchView, error)` | Новый, проброс. |
| HTTP | `GetMatchHandler() http.HandlerFunc` | Новый. 200/400/404/503. |
| Server | `r.Get("/matches/{id}", radarHTTP.GetMatchHandler())` | Новый роут внутри существующего `r.Route("/radar", …)`. |
| StoreAPI / mockStore | extend interface + mock | Обязательно. |

Никаких миграций, никаких новых пакетов.

## 2. Routes & navigation

| Path | File | Назначение |
|---|---|---|
| `/radar` | `radar._index.tsx` | Radar list: header + grid топик-карточек + last sweep + кнопка «New topic». Секции «On the radar» / «Paused». |
| `/radar/$topicId` | `radar.$topicId.tsx` | Topic view: header (name/description) + StatsLine + матчи infinite scroll + Edit/Pause/Delete actions. |
| `/radar/matches/$matchId` | `radar.matches.$matchId.tsx` | Match reader: layout как library reader, body = summary, CTA «Open original», action «Save to library», back-link на `/radar/$topicId`. |

Все три под `_app.tsx` (sidebar + topbar) и `ProtectedRoute` (как library).

Sidebar: в `Sidebar.tsx` снять `disabled: true` с Radar (line 6).

## 3. Data layer

### API-функции (`api.ts`)

```ts
listTopics(): Promise<TopicWithStats[]>
getTopic(id: number): Promise<TopicWithStats>
createTopic(input: { name: string; description: string; matchThreshold?: number }): Promise<TopicWithStats>
updateTopic(id: number, input: { name?: string; description?: string; isActive?: boolean; matchThreshold?: number }): Promise<TopicWithStats>
deleteTopic(id: number): Promise<void>
listMatches(args: { topicId?: number; state?: "new" | "seen"; limit: number; offset: number }): Promise<MatchList>
getMatch(id: number): Promise<MatchView>           // backend addition
updateMatch(id: number, input: { state: "new" | "seen" }): Promise<MatchView>
getStatus(): Promise<RadarStatus>
```

### Query keys

```ts
radarKeys = {
  all: ["radar"],
  topics: ["radar", "topics"],
  topic: (id) => ["radar", "topic", id],
  matches: (topicId, state) => ["radar", "matches", { topicId, state }],
  match: (id) => ["radar", "match", id],
  status: ["radar", "status"],
}
```

### Cache invalidation

| Mutation | Invalidate |
|---|---|
| `createTopic` | `topics` |
| `updateTopic` | `topics`, `topic(id)` |
| `deleteTopic` | `topics`; remove `topic(id)`, `matches(id, *)` |
| `markMatchSeen` | `matches(topicId, *)`, `topics` (newCount), `match(id)` |
| `useUpdateTopic` (pause/resume) | optimistic, rollback в `onError` |
| `useMarkMatchSeen` | optimistic, single field |

### Schemas

Backend отдаёт snake_case (`new_count`, `last_match_at`, `topic_id`, `match_threshold`, `is_active`, `discovered_at`, `feed_title`). Zod schemas декодят raw shape; map-функции конвертят в camelCase. Date-поля парсим в `Date | null`. `parseInDev` гейтит валидацию по DEV/test, как в library.

### Pagination

`useMatchesQuery` — infinite query, `pageParam` = offset, `PAGE_SIZE` = 20. `getNextPageParam` идентичен library-варианту.

## 4. Page components

### Radar list (`/radar`)

```
PageHeader        title="Radar"   subtitle=`Last sweep · ${fmt(status.lastSweepAt)}`
RadarToolbar      [+ New topic] (desktop right, mobile full-width below header)
<section>On the radar</section>
  TopicGrid (active=true)         grid-cols-1 md:grid-cols-2
<section>Paused</section>          (рендерится только если есть paused)
  TopicGrid (active=false, opacity-60)
```

`TopicCard` рендерит `TopicWithStats`:
- index-номер сверху-слева, `stats.newCount` справа (vermillion если > 0, dash если 0)
- name (display-tight 1.75rem)
- description (line-clamp-2)
- нижняя строка: `${stats.totalCount} found · ${stats.sourceCount} sources · ${fmt(stats.lastMatchAt)}`
- Link на `/radar/$topicId`

**Не выводим:** keyword-chips, sources list.

Loading: 4 `SkeletonCard`. Empty: `EmptyTopicList` (full-width CTA «+ New topic»).

«Awaiting first sweep» вместо timestamp'а, если `status.lastSweepAt === null`.

### Topic view (`/radar/$topicId`)

```
BackLink          ← Back to radar
TopicHeader       name, description, Edit/Pause/Delete справа (desktop)
StatsLine         <vermillion>{totalCount}</vermillion> found · {newCount} unread ·
                  {sourceCount} sources · created {fmt(createdAt)}
SectionHeader     "Found entries" · "{visibleCount} shown"
MatchGrid         infinite scroll, grid-cols-1 md:grid-cols-2 lg:grid-cols-3
```

`TopicHeader` actions:
- **Edit** → `EditTopicDialog` с текущими значениями
- **Pause/Resume** → `useUpdateTopic({ isActive: !current })`, optimistic toggle
- **Delete** → `DeleteTopicConfirm`, confirm → `useDeleteTopic` → navigate `/radar`

`MatchCard`:
- top строка: date · source · index
- title (display-tight, line-clamp-2)
- summary (line-clamp-3)
- vermillion stamp «new» в углу если `state === "new"`
- Link на `/radar/matches/$matchId`

Empty: `EmptyTopicMatches` — «Standing watch. New entries will appear here.»
404 топика → not-found route.

### Match reader (`/radar/matches/$matchId`)

```
BackLink          ← Back to {match.topicName}     (navigate to /radar/${match.topicId})
ReaderHeader      topic-stamp (link к топику), title, source · author · publishedAt · feedTitle
ReaderBody        summary (font-body, без drop-cap)
                  fallback «No summary captured. Open original to read.» если summary пуст
ReaderActions     [Open original ↗] (primary) · [Save to library] (secondary)
```

`useMatchQuery(matchId)` → mount-эффект: если `match.state === "new"` → `markSeen({ id, state: "seen" })`. Idempotent.

**«Save to library»** → `saveLink(match.finding.url)` (existing `library/api.ts`). Toast «Saved» + invalidate `["library"]`. Match остаётся в Radar как seen.

Layout-компоненты переиспользуем из library reader (`ReaderHeader`, `ReaderActions`). Если интерфейсы не совпадают, выносим shared ядро в `shared/layout/Reader*` и оборачиваем обоими.

Fallback'ы:
- пустой `finding.title` → URL вместо заголовка
- пустой `finding.author` / `finding.feedTitle` → пропускаем строку, не показываем плейсхолдер

### Dialogs

`NewTopicDialog` / `EditTopicDialog` — `shared/ui/dialog.tsx` (как `AddLinkDialog`):
- Поля: `name` (required, max 200), `description` (required, textarea, max 2000)
- Submit: `useCreateTopic` / `useUpdateTopic` → close + toast «Saved»
- 503 от embedder'а → toast «Embedder unavailable, retry later»; диалог НЕ закрывается, данные не теряются
- Generic ошибка → toast «Could not save»; dialog остаётся открытым

`DeleteTopicConfirm` — `shared/ui/alert-dialog.tsx`:
- Текст: «Delete topic "{name}"? Matches will be lost.» (backend каскадно дропает matches; findings не трогает)
- Confirm → `useDeleteTopic` → navigate `/radar`, toast «Deleted»

## 5. Data flow

Типичный путь:
1. `/radar` mount → `useTopicsQuery` + `useRadarStatusQuery` параллельно.
2. Клик по карточке → `/radar/$topicId` → `useTopicQuery(id)` + `useMatchesQuery({topicId: id})`. `topics` cache уже содержит этот топик от шага 1 — header рендерится мгновенно через placeholder/initialData.
3. Клик по матчу → `/radar/matches/$matchId` → `useMatchQuery(id)`. Если `match.state === "new"`, mount-эффект → `markSeenMutation`. Optimistic update в `matches(topicId, *)` и `topic.stats.newCount` (decrement) для мгновенного UI feedback'а в sidebar/list.
4. «Save to library» → `saveLink(finding.url)` → invalidate `["library"]`. Match остаётся seen.

### Mutations (общий паттерн)

- `mutationFn` → `onSuccess` (invalidate) + `onError` (toast).
- `useUpdateTopic`/`useCreateTopic` различают 503 (HTTP status или `error.code === "embedder_unavailable"`) от прочих.
- Optimistic update для `markMatchSeen` и `Pause/Resume`. Rollback в `onError`.

## 6. Edge cases & states

- **Direct URL на `/radar/matches/$matchId`** — `useMatchQuery` тянет match по id; BackLink использует `match.topicName` из ответа.
- **Удалённый топик с открытым reader** — следующий рефетч отдаёт 404 → not-found UI.
- **Пустой `last_sweep_at`** — «Awaiting first sweep» вместо timestamp'а.
- **Пустой `finding.summary`** — fallback-текст в reader body.
- **Пустой `finding.title`** — URL вместо заголовка.
- **`LINKTHECA_RADAR_ENABLED=false`** — backend отдаёт `501 radar_disabled` на `/radar/*` (через `DisabledHandler`). UI детектит по `error.code === "radar_disabled"` и рендерит full-page «Radar is disabled in this instance.» вместо grid'а. Sidebar-пункт виден (модуль присутствует в коде).
- **Embedder unavailable** — backend отдаёт `503 embedder_unavailable`. В Create/Edit dialog → specific toast; в других местах (например, фон) — generic toast.
- **401** — existing logout-flow (как library).

## 7. Accessibility

- Dialog (Radix-based) — focus-trap включён, escape закрывает, ARIA labels.
- Cards — wrapping `<Link>`, keyboard-accessible (Enter/Space через React Router).
- Icon-only action buttons (Edit/Pause/Delete на mobile) — `aria-label`.
- Status announcements: toast уже использует `sonner` (live region).

## 8. Testing

### Backend (Go, `internal/radar/`)

- `store_test.go` — `GetMatch` happy / другой user → 404 / не существует → 404
- `service_test.go` — mockStore проброс + ошибки
- `http_test.go` — 200 (JSON shape), 400 (bad id), 404, 503 disabled
- `integration_test.go` — добавить шаг `GET /radar/matches/{id}` в end-to-end сценарий
- Update `StoreAPI` interface + `mockStore`

### Frontend юнит (`features/radar/`)

- `api.test.ts` — fetch URL'ы, query-string для `listMatches`, body shape для `updateTopic` PATCH (только non-undefined поля)
- `schemas.test.ts` — Zod парсинг raw responses, snake_case → camelCase, null/optional поля
- `use-radar.test.tsx` — list, single, infinite pagination, error path
- `use-mutations.test.tsx`:
  - `useCreateTopic` + 503 → specific toast
  - `useUpdateTopic` optimistic + rollback
  - `useMarkMatchSeen` invalidation
  - `useDeleteTopic` navigate

### Компонентные тесты

- `TopicCard.test.tsx` — рендер stats, vermillion newCount, paused dimming, link href
- `NewTopicDialog.test.tsx` — submit, validation (empty name), 503 → dialog stays open, generic-error toast
- `EditTopicDialog.test.tsx` — initial values, partial-update body (только изменённые поля)
- Match reader **auto-mark-seen** — mount-эффект вызывает mutation ровно один раз при `state === "new"`, не вызывает при `state === "seen"` (критичный side-effect)

### Не тестируем

- Тривиальные компоненты (`StatsLine`, `EmptyTopicList`, `EmptyTopicMatches`) — covered косвенно.
- Layout-компоненты, переиспользованные из library — уже покрыты в library tests.

### Manual smoke (перед merge)

- Создать топик через UI → виден в списке → клик → Topic view, пустой → seed match через CLI → обновить → match виден → клик → reader → newCount уменьшился в списке → «Open original» открывает новую вкладку → «Save to library» → проверить в Library.
- Pause топика → переезжает в Paused-секцию → Resume → возвращается.
- Delete topic → подтверждение → редирект на `/radar`, матчей нет.
- Радикальный toggle: `LINKTHECA_RADAR_ENABLED=false` → UI показывает «Radar is disabled».

## 9. Out of scope (this iteration)

Зафиксировано здесь, чтобы при writing-plans не возвращаться к обсуждению:

- Keyword-chips в Topic UI и поле keywords в Create/Edit modal
- Sources block / per-topic feed picker
- Subscription management UI
- Threshold slider в Create/Edit
- Bulk «Mark all seen» в Topic view
- Full-content extraction для findings
- Cursor pagination
- Mobile-specific Radar layout сверх media queries from prototype