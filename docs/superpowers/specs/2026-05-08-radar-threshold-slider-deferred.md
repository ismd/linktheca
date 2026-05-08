# Per-topic match threshold slider with similarity preview (deferred)

**Status:** deferred — to be designed when Radar topic management UI gets built.
**Decided:** 2026-05-08.

## Context

`radar_topics.match_threshold` уже per-topic (REAL NOT NULL DEFAULT 0.55).
Сейчас пользователь не имеет UI-контроля: тема создаётся с дефолтом, и
пользователь не понимает, какие именно findings будут попадать в его
Radar до момента, пока что-то (или ничего) не упадёт.

Эмпирически на dev-данных видно, что **разные темы требуют разных
порогов**, потому что у BGE-M3 cosine similarity сжата в [0.4, 0.8]
и форма распределения зависит от ширины темы:

| Тема (description) | Шумовой потолок | Реальные совпадения |
|---|---|---|
| AI («machine learning research and large language models») | ~0.48 | 0.45–0.55 (Gemma, LLM benchmarks, AI agents) |
| WebAuthn («webauthn passkeys») | ~0.49 | 0.59 (точечные совпадения) |
| Wolfenstein («Wolfenstein 3D for Gameboy») | ~0.32 | 0.75 (прямое лексическое) |

Широкие темы (AI) → много пограничных кейсов в [0.5, 0.55), пользователь
скорее хочет порог 0.50.
Узкие термины (WebAuthn) → ниже 0.55 идёт чистый шум, порог 0.55+
правильнее.
Лексические совпадения (Wolfenstein) → нечувствительны к порогу.

Без визуальной обратной связи угадать «правильный» порог пользователь
не сможет — sim 0.55 ничего не значит без контекста распределения по
его данным.

## Целевой UX

Слайдер «Match threshold» (0.40–0.90, шаг 0.01 или 0.05) на странице
редактирования темы. Рядом — live preview списка top-N findings с
их sim-score, упорядоченных по убыванию. При движении слайдера в
preview визуально видно, что отвалится / что добавится:

```
Match threshold: ▓▓▓▓▓▓▓▓░░░░ 0.55

Would match (current setting):
  0.75  Wolfenstein 3D for Gameboy Color on custom cartridge (2016)
  0.59  WebAuthn и Passkeys — аутентификация без паролей
  ───────────────────────────── threshold ─────────────────────────────
  0.54  A Theory of Deep Learning                       (would NOT match)
  0.53  ProgramBench: Can Language Models Rebuild...    (would NOT match)
  0.50  Show HN: Adam – embeddable AI agent library     (would NOT match)
  0.48  Three Inverse Laws of AI                        (noise zone)
  0.47  Telus Uses AI to Alter Call-Agent Accents
  ...
```

Это превращает абстрактное «0.55 vs 0.50» в конкретную картину
«вот эти 4 статьи добавятся, если опустить порог».

## Что нужно сделать, когда будем реализовывать

### 1. Backend endpoint

`GET /radar/topics/{id}/preview?limit=20` — возвращает топ-N findings,
отсортированные по sim против embedding темы:

```json
{
  "findings": [
    {"id": 18, "title": "...", "sim": 0.7497, "would_match": true},
    {"id": 452, "title": "...", "sim": 0.5933, "would_match": true},
    {"id": 275, "title": "...", "sim": 0.5380, "would_match": false},
    ...
  ],
  "current_threshold": 0.55
}
```

SQL — то же выражение, что в `MatchFindingToTopics`, но без фильтра по
`match_threshold` и с LIMIT:

```sql
SELECT f.id, f.title, 1 - (rt.embedding <=> f.embedding) AS sim
FROM radar_topics rt
JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
JOIN radar_findings f ON f.feed_id = rfs.feed_id
WHERE rt.id = $1
  AND rt.embedding IS NOT NULL AND f.embedding IS NOT NULL
ORDER BY sim DESC
LIMIT $2;
```

Стоимость: один топ-N запрос на каждое перетаскивание — слишком часто.
Кэшировать список на клиенте, при движении слайдера фильтровать
in-memory; запрос делать только при смене темы / открытии страницы.

### 2. Frontend

Слайдер + список с разделителем «threshold cutoff line». При движении
слайдера разделитель скользит по списку; визуально подсвечиваются
findings выше / ниже. По «Save» — `PATCH /radar/topics/{id}` с новым
threshold.

### 3. Опциональное расширение: tier guidance

Использовать данные распределения, чтобы предлагать «рекомендуемые»
зоны прямо на слайдере:

- зелёная зона (high precision): выше пика «true positives»;
- жёлтая зона (gray area): где шум начинает примешиваться;
- красная зона (high recall, много false positives).

Для этого нужно понимание, где у конкретной темы шумовой потолок —
эмпирически это ~p95 или ~p98 sim-распределения по всем findings
этой темы.

## Зависимости

- Topic editing UI вообще (сейчас тем нет в UI, только админ-API).
- Достаточное количество findings в системе, чтобы превью было
  информативным (на 5 findings слайдер бесполезен).

## Когда возвращаться

Когда:
1. Будет реализована страница «My Topics» (фаза 3b или позже), и
2. У типичного пользователя будет хотя бы ~50–100 findings, чтобы
   распределение sim было видно.

До тех пор пользователи могут менять threshold через прямой PATCH
на API (если он добавится) или вручную через SQL для dev-сетапов.

## Связанные документы

- `docs/superpowers/specs/2026-05-06-embedding-model-decision.md` —
  выбор bge-m3 и его сжатый диапазон cosine similarity, который и
  делает unified default невозможным.
- `internal/radar/service.go` — `defaultMatchThreshold = 0.55` как
  стартовая точка.
- `internal/radar/store.go` `MatchFindingToTopics` — SQL для матчинга,
  на основе которого делается preview-запрос.