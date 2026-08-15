# Radar Sources UX — design

**Status:** approved, ready for implementation plan.
**Date:** 2026-08-15.

## Проблема

Ленты (`radar_feeds`) сейчас заводятся только админом через API, а подписки
(`radar_feed_subscriptions`) оформляются единственным эндпоинтом
`POST /radar/subscriptions`. В UI нет ничего: юзер не видит каталог, не может
подписаться или отписаться, админ не может поправить или убрать ленту. Radar
работает только у того, кто ходит в него curl'ом.

## Границы

**В объёме:** каталог лент, видимый всем юзерам; подписка и отписка; админские
add / edit / disable / delete; автоподписка нового юзера на весь активный
каталог; автозаполнение названия ленты краулером.

**Вне объёма:** личные ленты пользователей и квоты под них
(`docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` остаётся в
силе, колонка `created_by` не добавляется); HTML-скрапинг не-RSS источников и
авто-обнаружение feeds на сайте; ручной «Fetch now»; per-topic источники
(решено ранее: подписки per-user, не per-topic).

## Решения

1. **Один экран `/radar/sources`** в разделе Radar. Админские действия
   встроены в те же строки, отдельной админки нет.
2. **Отписка не трогает прошлое.** Новых матчей из ленты не будет, уже
   найденные остаются в Inbox и в топиках.
3. **Новый юзер подписывается на весь активный каталог** при регистрации.
   Ленты, добавленные позже, никому не навязываются.
4. **Один read-эндпоинт `GET /radar/feeds`** для всех ролей, с полем
   `subscribed` на строку. Запись остаётся под `RequireAdmin`.
5. **Название ленты подхватывает краулер**, ручная правка админа приоритетнее.

## Схема БД

Миграций нет. `radar_feeds → radar_findings → radar_topic_matches` уже связаны
`ON DELETE CASCADE`, так что удаление ленты чистит находки и матчи само.

## API

| Метод | Путь | Доступ | Поведение |
|---|---|---|---|
| `GET` | `/radar/feeds` | user | Каталог. Пагинация как сейчас (`limit` ≤ 100, default 50). Сортировка меняется с `created_at DESC` на `lower(coalesce(title, url)) ASC`. |
| `POST` | `/radar/subscriptions` | user | Без изменений; идемпотентен через `ON CONFLICT`. |
| `DELETE` | `/radar/subscriptions/{feedId}` | user | 204. Отписка от неподписанной ленты — не ошибка. |
| `POST` | `/radar/feeds` | admin | Без изменений. Дубль URL → 409 `duplicate`. |
| `PATCH` | `/radar/feeds/{id}` | admin | `title`, `fetch_interval_seconds`, `is_active` — все опциональны. |
| `DELETE` | `/radar/feeds/{id}` | admin | 204, cascade. |

Строка каталога — `FeedListItem`: текущий `Feed` плюс

- `subscribed bool` — есть ли подписка у юзера из контекста;
- `finding_count int` — сколько находок у ленты; нужен для confirm-диалога
  удаления.

`last_fetched_at` и `last_error` видны всем: для юзера это единственное
объяснение, почему подписанный источник молчит. Инстанс self-hosted, юзеры
доверенные.

### Ошибки

`ErrInvalidInput` → 400, не-админ на write → 403 (middleware),
`ErrNotFound` → 404, `ErrDuplicate` → 409, `ErrFeedNotFound` → 404 при подписке
на удалённую ленту. Гонка «админ удалил ленту, пока юзер жал чекбокс» решается
сама: подписка отвечает 404, оптимистичное обновление откатывается, список
инвалидируется.

## Бэкенд

### Матчинг

`Store.MatchFindingToTopics` уже джойнит `radar_feed_subscriptions`, поэтому
удаление строки подписки само останавливает новые матчи, а старые не трогает.
Выбранная семантика отписки не требует новой логики.

### Автоподписка при регистрации

`auth.ServiceConfig` получает поле

```go
OnUserCreated func(ctx context.Context, userID int64)
```

без возврата ошибки. `Service.Register` вызывает его после успешного
`CreateUser`, если поле не nil. Политикой ошибок владеет вызывающая сторона: в
`server.go` замыкание дёргает `radarSvc.SeedSubscriptions` и само логирует
неудачу через `deps.Logger`. Регистрация не падает — недоступный Radar не
должен блокировать вход в продукт. Хук ставится только внутри ветки
`cfg.RadarEnabled`; при выключенном Radar это nil.

Сидинг — один запрос:

```sql
INSERT INTO radar_feed_subscriptions (user_id, feed_id)
SELECT $1, id FROM radar_feeds WHERE is_active
ON CONFLICT DO NOTHING
```

Общей транзакции с созданием юзера нет. В худшем случае юзер стартует без
подписок и чинит это галочками. Известное следствие: зарегистрировавшийся при
выключенном Radar автоподписки не получит — ловим это empty-state'ом на экране
Sources, не миграцией.

### Название ленты

Сегодня `radar_feeds.title` не заполняет никто: `Store.AddFeed` его не пишет,
`crawler.Parse` возвращает только `feed.Items` и выбрасывает заголовок канала,
`MarkFeedFetched` колонку не трогает. Поэтому в живой базе он всегда NULL — и
карточки матчей уже сейчас подписаны хостами (`MatchCard.tsx:22` делает
`feedTitle ?? host(url)`).

Чиним в рамках этой фичи:

- `crawler.Parse` возвращает заголовок канала вместе с элементами (структура
  `ParsedFeed{Title string; Items []*gofeed.Item}` вместо голого слайса);
- `CrawlFeed` (`internal/radar/jobs/crawl_feed.go`) передаёт его в
  `MarkFeedFetched`, который пишет `title = COALESCE(radar_feeds.title, $n)` —
  то есть заполняет только пустое.

Ручное название админа приоритетнее и никогда не затирается автоподхватом.
Очистка поля в `EditFeedDialog` (`title → null`) возвращает автоматическое имя
на следующем обходе. Ветка 304 Not Modified названия не трогает.

### Новое в `internal/radar`

- `Service.SeedSubscriptions(ctx, userID) error`
- `Service.Unsubscribe(ctx, userID, feedID) error`
- `Service.UpdateFeed(ctx, feedID, req) (*Feed, error)`
- `Service.DeleteFeed(ctx, feedID) error`
- `Service.ListFeeds` меняет сигнатуру — принимает `userID` для `subscribed`.
- Валидация интервала выносится из `AddFeed` в `validateFetchInterval(int) error`
  и переиспользуется в `UpdateFeed`. Пустой патч → `ErrInvalidInput` → 400,
  как у топиков.
- `Store.UpdateFeed` — по образцу `Store.UpdateTopic`: динамический набор
  `SET`-клауз, `RETURNING`, 0 строк → `ErrNotFound`.
- `Store.ListFeeds` — `EXISTS (SELECT 1 FROM radar_feed_subscriptions …) AS
  subscribed` и `LEFT JOIN` с агрегатом по `radar_findings` для
  `finding_count` (одним запросом, не N+1).
- `Store.Unsubscribe`, `Store.DeleteFeed` — простые `DELETE`; первый молчит на
  0 строк, второй возвращает `ErrNotFound`.

### Роутинг

`GET /radar/feeds` и `DELETE /radar/subscriptions/{feedId}` живут в общей
user-группе. Под `RequireAdmin` остаются `POST`, `PATCH`, `DELETE`
`/radar/feeds`.

## Фронтенд

Роут `web/src/routes/radar.sources.tsx`, регистрируется в `App.tsx` как
`radar/sources`. Вход — ссылка `Sources →` в `PageHeader` рядом с существующей
`Topics →` на инбоксе и на списке топиков. Пункт в `Sidebar` не добавляется:
Radar остаётся одной строкой навигации.

```
Sources                                    [+ Add feed]   ← admin
Changes apply from the next sweep.

☑  The Verge                                  [edit] [✕]  ← admin
   theverge.com/rss · every 1h · fetched 4m ago · 214 items
──────────────────────────────────────────────────────────
☐  Ars Technica                               [edit] [✕]
   arstechnica.com/feed · every 1h · ⚠ 404 Not Found (2d ago) · 61 items
──────────────────────────────────────────────────────────
☑  Hacker News                                [edit] [✕]
   news.ycombinator.com/rss · paused · 1 203 items
```

`SourceRow`: слева нативный чекбокс подписки, стилизованный под текущую
типографику (новый примитив в `shared/ui` не заводим), в центре заголовок и
мета-строка, справа админские действия. Заголовок — `title`, при null —
hostname из URL. Мета-строка склеивается из интервала, состояния последней
выборки (`fetched Nm ago` / `⚠ <last_error> (Nd ago)` / `never fetched`) и
`paused` при `is_active=false`. Приостановленная лента приглушается, но
чекбокс остаётся рабочим — подписаться заранее не вредно.

### Диалоги

На существующих `dialog.tsx` / `alert-dialog.tsx`, по образцу `NewTopicDialog`
и `DeleteTopicConfirm`.

- `AddFeedDialog` — URL и селект интервала (30m / 1h / 3h / 6h / 12h / 24h).
  `kind` в UI не выносится, бэкенд ставит `rss`. 409 показывается инлайном:
  «This feed is already in the catalog».
- `EditFeedDialog` — title (пусто → `null`, автоматическое имя вернётся на
  следующем обходе), интервал, тумблер Paused.
- `DeleteFeedConfirm` — «Delete *The Verge*? 214 findings and their matches
  will be removed for all users.» Число берётся из `finding_count`.

### Состояние

Ключ `radarKeys.feeds`. Подписка и отписка — оптимистичные: флипаем
`subscribed` в закэшированном списке, откатываем на ошибке (паттерн из
`use-mutations.tsx`). Админские мутации инвалидируют список. Ошибки — тостом
через подключённый `sonner`.

### Пустые состояния

Каталог пуст: админу — «Add the first feed» с кнопкой, обычному юзеру —
«No sources yet. Ask the instance admin to add feeds.» Ответ `radar_disabled`
рендерит `RadarDisabled`, как на остальных радар-роутах.

Админские контролы прячутся по `isAdmin` из сессии. Это косметика; правду
говорит `RequireAdmin` на бэкенде.

## Тестирование

Разработка по TDD: тест вперёд, потом реализация.

**Go, unit (`http_test.go`, мок-стор):** PATCH меняет только переданные поля;
пустой патч → 400; интервал вне 300..86400 → 400; DELETE несуществующей ленты
→ 404; `GET /radar/feeds` отдаёт `subscribed` по юзеру из контекста;
unsubscribe идемпотентен (204 дважды).

**Go, краулер и jobs:** `Parse` отдаёт заголовок канала; `MarkFeedFetched`
заполняет пустой `title` и не затирает уже заданный; ветка 304 название не
меняет.

**Go, интеграционные (`integration_test.go`, реальная БД):** subscribe →
находка матчится → unsubscribe → новая находка не матчится, старый матч на
месте; удаление ленты каскадом сносит findings и matches; `SeedSubscriptions`
подписывает только на активные ленты и идемпотентен.

**Go, auth и server:** `Register` дёргает `OnUserCreated` с id созданного
юзера; при nil-хуке регистрация работает как раньше; при `RadarEnabled=false`
хук не ставится.

**Фронт (vitest + RTL + msw):** оптимистичный тумблер подписки и откат на 500;
админские контролы не рендерятся при `isAdmin: false`; confirm удаления
показывает `finding_count`; 409 в `AddFeedDialog` выводится инлайном; пустой
каталог даёт разный текст админу и юзеру.
