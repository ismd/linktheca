# Settings Screen — Design (read-only, frontend-only)

**Дата:** 2026-06-04
**Статус:** утверждён, готов к написанию плана
**Связано с:** прототип `prototype/index.html` (секция Settings), `2026-05-18-radar-frontend-design.md` (паттерн feature-модуля, `useRadarStatusQuery`)

---

## Цель

Заменить заглушку `web/src/routes/settings.tsx` («Coming soon») на реальный экран **только для чтения**. Это закрывает последний оставшийся stub в сайдбаре (`/settings`, пункт «03») и завершает поверхность MVP-UI.

**Никаких изменений на бэкенде.** Экран потребляет только то, что API уже отдаёт сегодня: `useAuthStore` (заполнен из `GET /auth/me`) и `GET /radar/status`.

## Не-цели (явно вне объёма)

Эти пункты прототипа сознательно **не** реализуются в этой итерации:

- Редактирование display name / email / пароля (нет backend-endpoint'ов; отдельная фича «Editable Account»).
- API-токены.
- Appearance-настройки (theme, drop caps, paper grain, progress bar) — нет персистентности и нет влияния на reader.
- Notifications (browser / email digest / ntfy) — вне MVP-scope по архитектуре.
- Monitoring / Parsing config (refresh interval, embedding model, threshold, max sources, user-agent) — env-уровень и/или отложенные фичи (per-topic threshold — отдельная отложенная фича).
- Instance-метрики: hostname, uptime, database size, storage, export archive, view logs — нет источника данных.

Если позже понадобится редактируемый Account — это новая брейншторм-сессия со своим spec и backend-задачами.

## Решения, зафиксированные при брейншторме

| Вопрос | Решение |
|---|---|
| Объём итерации | Read-only, frontend-only. Ноль нового backend. |
| Набор секций | Две: **Account** и **About**. |
| Структура | Одна колонка, секции-карточки стопкой. Без левого section-nav (избыточен для 2 секций). Зеркалит паттерн Library/Radar. |
| Logout | Не дублируем — уже есть в `UserMenu` (header). Account read-only, без действий. |
| Источник версии | Статическая константа (package.json `version` не поддерживается). Выносим в `shared/version.ts`. |

---

## Архитектура

### Размещение

Лёгкий feature-модуль `web/src/features/settings/` — **без** `api.ts`/`schemas.ts`/собственных хуков, потому что данных своих нет: Account берётся из `useAuthStore` (`features/auth`), Radar-статус — из `useRadarStatusQuery` (`features/radar`). Это допустимый cross-feature consume (Settings — сквозной потребитель, а не нарушение границы Library/Radar).

### Файлы

| Path | Изменение |
|---|---|
| `web/src/shared/version.ts` | create — `export const APP_VERSION = "0.1.0";` |
| `web/src/features/settings/components/SettingRow.tsx` | create — презентационная строка `label · value` |
| `web/src/features/settings/components/AccountSection.tsx` | create |
| `web/src/features/settings/components/AccountSection.test.tsx` | create |
| `web/src/features/settings/components/AboutSection.tsx` | create |
| `web/src/features/settings/components/AboutSection.test.tsx` | create |
| `web/src/routes/settings.tsx` | rewrite — header + `<AccountSection />` + `<AboutSection />` |
| `web/src/routes/settings.test.tsx` | create — рендерит оба заголовка секций |
| `web/src/shared/layout/Sidebar.tsx` | modify — заменить хардкод `v0.1.0` на `APP_VERSION` |

---

## Компоненты

### `SettingRow`

Презентационный, без состояния. Рисует строку: маленький uppercase-label слева/сверху и значение `text-ink`. Зеркалит `settingRow` из прототипа, но без editable-кнопок (всё read-only).

```
Props: { label: string; value: ReactNode }
```

Используется и в Account (Email, Role), и в About (Version, Mode, Radar).

### `AccountSection`

```tsx
const user = useAuthStore((s) => s.user);
if (!user) return null; // ProtectedRoute гарантирует authed, но защищаемся
const initial = user.displayName.charAt(0).toUpperCase() || "·";
const role = user.isAdmin ? "Administrator" : "Member";
```

Рендер:
- Аватар-инициал: квадрат `w-16 h-16 bg-ink text-paper font-mono` (тот же паттерн, что в `UserMenu`).
- `display-tight` имя (`user.displayName`).
- `SettingRow` Email = `user.email`.
- `SettingRow` Role = `role`.

Без кнопок Edit/Change (редактирование вне объёма — не показываем нерабочие контролы).

### `AboutSection`

```tsx
const status = useRadarStatusQuery();
```

Рендер трёх `SettingRow`:
- **Version** = `v{APP_VERSION}` (→ `v0.1.0`).
- **Mode** = `self-hosted` (статика).
- **Radar** = строка по состоянию query (см. ниже).

#### Маппинг состояния Radar (единственная динамика на экране)

| Состояние query | Значение строки Radar |
|---|---|
| `isLoading` | `Checking…` |
| `isSuccess` | `fmtSweep(data.lastSweepAt)` → `Last sweep · 2h ago` или `Awaiting first sweep` |
| `isError` && `error instanceof ApiError` && `error.code === "radar_disabled"` | `Disabled` |
| любой другой error | `Unavailable` |

`fmtSweep` импортируется из `@/features/radar/time`. Проверка `radar_disabled` зеркалит `routes/radar._index.tsx` (`error.code === "radar_disabled"`). `ApiError` имеет поля `status`, `code`.

### `routes/settings.tsx`

Переписывается со stub на:

```tsx
<div>
  <PageHeader title="Settings" subtitle="This instance and your account." />
  <AccountSection />
  <AboutSection />
</div>
```

Подзаголовок `This instance and your account.` — конкретное значение по умолчанию; допустима замена на эквивалент в том же editorial-тоне.

Каждая секция — карточка `bg-paper-2 border border-rule p-6 md:p-8` с `display-tight` заголовком и italic-подзаголовком (идиома прототипа `settingsSection` и существующих экранов Library/Radar). Точная вёрстка заголовка (`PageHeader` vs кастомный wonky-header как в прототипе) — на усмотрение реализации, в рамках существующего layout-паттерна.

---

## Поток данных

```
AccountSection ── useAuthStore (заполнен auth bootstrap/login) ── displayName, email, isAdmin
AboutSection ──┬─ APP_VERSION (shared/version.ts) ─────────────── Version, Mode
               └─ useRadarStatusQuery → GET /radar/status ──────── Radar row
Sidebar ─────── APP_VERSION (shared/version.ts) ───────────────── footer-строка
```

## Обработка ошибок

- **Account:** guard `!user → null`. На защищённом маршруте user всегда есть; guard на случай гонки при разлогине.
- **About / Radar:** строка никогда не бросает — каждое состояние query (loading / success / disabled / other-error) маппится в строку. Экран рендерится полностью даже при выключенном Radar (консистентно с тем, что сайдбар оставляет Radar видимым при `radar_disabled`).

## Тестирование

Vitest + Testing Library + MSW, по образцу radar-тестов: `QueryClient`-wrapper с `retry: false`, `useAuthStore.getState().setSession(...)` в `beforeEach`, `server.use(http.get(...))` для `/api/radar/status`.

- **`AccountSection.test.tsx`**
  - authed session (`isAdmin: false`) → видны `displayName`, `email`, `Member`.
  - authed session (`isAdmin: true`) → видно `Administrator`.
- **`AboutSection.test.tsx`**
  - `/radar/status` → `{ last_sweep_at: "<iso>" }` → видны `v0.1.0`, `self-hosted`, и текст sweep (`Last sweep · …`).
  - `/radar/status` → ошибка с `code: "radar_disabled"` → видно `Disabled`.
- **`settings.test.tsx`**
  - маршрут рендерит заголовки обеих секций: `Account` и `About`.

## Стиль

Editorial-идиома проекта: карточки `bg-paper-2 border border-rule`, `display-tight` заголовки секций, italic-подзаголовки, `label-sc`/`font-mono` для меток и значений — как в прототипе (`settingsSection`, `settingRow`) и на готовых экранах Library/Radar. Никаких новых дизайн-примитивов.

---

## Spec coverage

| Что | Где покрыто |
|---|---|
| `shared/version.ts` + рефактор Sidebar | Архитектура → Файлы |
| `SettingRow` | Компоненты |
| Account (identity, role, read-only) | Компоненты → AccountSection |
| About (version, mode, radar) | Компоненты → AboutSection |
| Состояния Radar (loading/success/disabled/error) | Маппинг состояния Radar |
| Route-композиция | routes/settings.tsx |
| Тесты | Тестирование |
| Явные не-цели | Не-цели |

Backend не затрагивается. Все источники данных существуют сегодня.
