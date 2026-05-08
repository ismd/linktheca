# Radar full-content extraction (deferred)

**Status:** deferred — to be designed alongside reader view for findings.
**Decided:** 2026-05-08.

## Context

Сейчас Radar embedder строит вектор по `title + summary`, где `summary`
это `<description>` из RSS. Многие агрегаторы (HN, Lobsters, Reddit)
кладут туда заглушку — `<a>Comments</a>`, ссылку на обсуждение или
просто URL. В фазе 3a-2 такие описания отсекаются на уровне crawler'а
(`sanitizeSummary` в `internal/radar/crawler/crawler.go`), и для подобных
лент эмбеддинг строится только по title.

Это работает, но качество тематического матчинга для агрегаторов
остаётся ограниченным: title — это заголовок, без контекста статьи.
Полноценное решение — отдельный шаг пайплайна, который ходит по
`finding.url`, извлекает контент и подаёт его в embedder.

Колонка `radar_findings.content_id BIGINT NULL REFERENCES article_contents(id)`
уже зарезервирована под это (миграция 009) и сейчас всегда NULL.

## Целевой пайплайн

```
Scheduler → CrawlFeed → FetchContent → EmbedFinding → MatchFinding
                          ↑ NEW
```

`FetchContent` берёт `finding.url`, извлекает основной текст,
сохраняет в `article_contents`, проставляет `radar_findings.content_id`.
`EmbedFinding` после этого предпочитает текст из `article_contents`
вместо `summary`.

## Что нужно сделать, когда будем реализовывать

### 1. Job `FetchFindingContent`

Новый river worker в `internal/radar/jobs/fetch_finding_content.go`.
Аргумент: `FindingID int64`. Шаги:

1. Загрузить finding (для url + проверки, что content_id уже не стоит).
2. HTTP GET с тайм-аутом и лимитом размера (как `HTTPFetcher` в crawler'е).
3. Прогнать HTML через extractor (см. ниже).
4. Если контент извлечён — `INSERT INTO article_contents` + `UPDATE radar_findings SET content_id`.
5. Enqueue `EmbedFinding`. (Сейчас это делает `CrawlFeed`; надо перенести.)

### 2. Extractor

Варианты:

- **`go-readability`** (`github.com/go-shiori/go-readability`) — порт
  Mozilla Readability на Go, поддерживается, без CGO. Хорошо
  справляется с типичными статьями.
- **`go-trafilatura`** — порт trafilatura, лучше работает с новостными
  сайтами, но шумнее по зависимостям.
- **Кастомный наивный strip** — нет, слишком много кейсов
  (paywall, cookie banners, JS-only).

Дефолт: `go-readability`. Запасной путь, если экстрактор вернул
пусто — оставляем `content_id = NULL`, embedder работает по title.

### 3. Robots.txt и rate-limit

Открытый fetch URL'ов из произвольных RSS = воспроизведение
изначальной DoS-проблемы (см. `2026-05-06-user-added-feeds-deferred.md`),
плюс легальные/этические соображения.

Минимум:

- Уважать `robots.txt` (например, `github.com/temoto/robotstxt`),
  кэшируя per-host на сутки.
- Per-host token bucket (например, не больше 1 RPS на хост).
- Тайм-аут запроса ≤ 15 секунд, лимит размера ≤ 5 MiB.
- User-Agent с контактом (как Wayback / Common Crawl делают).

### 4. Хранение

`article_contents` уже существует от Library. Схема общая для Library
и Radar — оба «складывают» извлечённый текст в одну таблицу. Стоит
проверить:

- Есть ли уникальность по url? Если нет — добавить, чтобы один и
  тот же URL не извлекался дважды (Library и Radar).
- Размер контента: BLOB / TEXT? Если ограничения нет, добавить
  CHECK на максимальный размер (~1 MiB после очистки).

### 5. Обработка ошибок

`FetchContent` не должен валить весь пайплайн при единичной ошибке.
Семантика:

- 4xx (404, 403) → перманентная ошибка, не ретраить, оставить NULL,
  залогировать.
- 5xx / network → river retry с backoff (стандартный механизм).
- robots.txt запрещает → не fetcher, не error — просто оставить NULL.
- extractor вернул пусто / слишком короткий текст → оставить NULL.

### 6. Embedder

`internal/radar/jobs/embed_finding.go` — `embedText`. Новая логика:

```
if finding.content_id != NULL:
    text = article_contents.content (cap, например, 4096 токенов)
else:
    text = title + summary
```

Cap по токенам нужен, потому что bge-m3 имеет limit ≈ 8192, и кормить
длинные статьи целиком неэффективно (TEI медленно, эмбеддинг шумный
от объёма второстепенного контента).

## Когда возвращаться

К этому документу — когда будет реализовываться reader view для
findings (фаза 3b или позже): для просмотра тоже нужен извлечённый
текст, и было бы расточительно делать extraction дважды.

До тех пор `sanitizeSummary` в crawler'е достаточно для того, чтобы
убрать самый явный шум из эмбеддингов агрегаторных лент.

## Связанные документы

- `docs/superpowers/specs/2026-05-06-embedding-model-decision.md` —
  выбор bge-m3 и token-cap.
- `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md` —
  квоты для пользовательских лент (та же DoS-история, что и здесь).