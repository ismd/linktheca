# User-added Radar feeds (deferred)

**Status:** deferred — to be designed alongside the Radar Settings UI.
**Decided:** 2026-05-06.

## Context

В фазе 3a-2 `POST /radar/feeds` стоит под `RequireAdmin`. Это сознательный
выбор на время становления пайплайна: feeds — инстанс-уровневый ресурс,
открытое добавление без квот = DoS-вектор для краулера и TEI.

## Целевой UX

Согласовано следующее видение:

- Существует **общий курируемый каталог feeds**, который ведёт админ
  (`POST /radar/feeds` под `RequireAdmin` — текущее поведение).
- Каждый пользователь имеет **свой набор подписок**: галочками
  включает/отключает любые ленты из каталога
  (`POST/DELETE /radar/subscriptions` — текущее поведение).
- Каждый пользователь дополнительно может **добавить свою личную ленту**
  в каталог. Дедупликация по URL: если такой URL уже есть, юзер просто
  получает на него подписку. Это новая функциональность.

## Что нужно сделать, когда будем реализовывать

### 1. Endpoint для пользовательского добавления

Варианты:
- Снять `RequireAdmin` с `POST /radar/feeds` и различать поведение по
  роли (admin → curated, обычный юзер → personal + auto-subscribe).
- Или отдельный `POST /radar/feeds/personal` (чище семантически).

При добавлении: если URL уже существует в `radar_feeds` — не создаём
новую запись, просто оформляем subscription. Если нет — создаём запись
+ subscription одной транзакцией.

### 2. Колонка `created_by`

Добавить в `radar_feeds`:

```sql
ALTER TABLE radar_feeds
  ADD COLUMN created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
```

Семантика:
- `NULL` — curated (добавлено админом / системный seed). Все
  существующие записи на момент миграции = curated, backfill не нужен.
- `not NULL` — добавлено конкретным пользователем (personal feed).

Используется в UI для разделения «каталог» vs «мои ленты», и для
квот (см. ниже).

### 3. Квоты (admin-configurable)

Без квот открытый endpoint = DoS. Минимально:

- **Per-user limit на personal feeds**: количество записей в
  `radar_feeds` с `created_by = $user_id`. Дефолт, например, 20.
- **Per-feed minimum interval**: нижняя граница `fetch_interval_seconds`,
  чтобы юзер не выставил 60s на десяток лент. Дефолт, например, 1800s.
- **Global rate-limit**: общее количество ленты в инстансе.

Все три значения — это конфиг инстанса, который выставляет админ.
Хранение: либо `instance_config` таблица (key/value), либо env-vars.
Обсуждаемо в момент реализации.

### 4. Админский UI

- Promote: пометить personal feed как curated (`created_by → NULL`).
- Demote: обратное действие.
- Список с фильтром curated / personal.
- Изменение квот.

### 5. Удаление

Когда юзер «удаляет свою ленту»:
- Если на неё подписан только он — удалить feed целиком (cascade удалит
  findings и matches).
- Если есть другие подписчики (например, после promote-to-curated и
  затем demote) — только отписать юзера, feed оставить.

## Когда возвращаться

К этому документу — на этапе проектирования Radar Settings UI
(предположительно фаза 3b или позже). До тех пор текущей admin-only
модели достаточно для всех smoke-тестов и dogfood-сценариев.