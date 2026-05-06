# Решение: модель embeddings — `BAAI/bge-m3`

**Дата:** 2026-05-06
**Статус:** approved
**Уточняет:**
- `2026-04-10-architecture-design.md` — раздел 6 «Embeddings»
- `2026-04-22-phase-3a-radar-pipeline-design.md` — раздел «Контекст», п. 1

## TL;DR

Linktheca использует **`BAAI/bge-m3`** (1024 dim, MIT) на сервере **HuggingFace Text Embeddings Inference (TEI)** для всех embeddings проекта (Radar topics, Radar findings). Модель выбрана как Pareto-оптимум по жёстким ограничениям: self-hosted, бюджет RAM, мультиязычность с cross-lingual retrieval, permissive-лицензия, dim = 1024 (зафиксирован в pgvector schema), нативная поддержка в TEI.

Альтернативы (`bge-multilingual-gemma2`, `Qwen3-Embedding-*`, `multilingual-e5-large`, `Llama-Embed-Nemotron`, `gemini-embedding-001` и др.) рассмотрены и отклонены — см. таблицу ниже.

## Контекст

Linktheca — open-source on-premise сервис; пользователи деплоят его на свои машины и читают контент на разных языках. Модуль Radar выполняет semantic retrieval: тема пользователя (на любом языке) ищет статьи из RSS на любом языке. Качество cross-lingual retrieval и широта языкового покрытия определяют пользу Radar для международной аудитории.

Размерность embedding'а — **долгосрочное архитектурное решение**: смена модели на другую dim потребует миграции pgvector schema, пересчёта всех существующих embeddings и пересборки HNSW-индексов. Цена ошибки на этом уровне высокая, поэтому решение оформляется отдельным документом.

## Жёсткие ограничения

| # | Требование | Источник |
|---|---|---|
| **R1** | Self-hosted, без cloud API для embeddings | `CLAUDE.md`; решение «embeddings локально, LLM — opt-in cloud» зафиксировано в memory `project_ai_stack_decision.md` |
| **R2** | RAM ≤ ~3 GB для embedding-сервиса | Целевое железо: домашний сервер / VPS пользователя, не дата-центр |
| **R3** | Multilingual + **cross-lingual retrieval** (тема на одном языке → находит статьи на других) | Международная аудитория open-source проекта; без cross-lingual'а тематический мониторинг сводится к моноязычному поиску |
| **R4** | Permissive-лицензия (MIT/Apache 2.0) | Пользователи деплоят на свои машины — нельзя подвязывать их под restrictive TOS вендора |
| **R5** | dim = 1024 | Уже зафиксировано в pgvector schema (`vector(1024)` в миграциях `006_radar_topics`, `009_radar_findings`) и в HNSW-индексах |
| **R6** | Контекст ≥ 4K токенов | Title + description + summary статей бывают длинными, особенно у языков с длинными словоформами (финский, немецкий) |
| **R7** | Совместимость с TEI image `cpu-1.9` | Уже подключено в `compose.dev.yaml`, протестировано в `internal/core/embeddings/client_smoke_test.go` |

## Сравнение кандидатов

| Модель | Dim | RAM | Multilingual | License | TEI | Решение |
|---|---|---|---|---|---|---|
| **`BAAI/bge-m3`** | **1024** | ~2.5 GB | ✅ 100+ langs, MIRACL nDCG@10 ≈ 70.0 (комбо-режим) | **MIT** | ✅ XLM-RoBERTa native | ✅ **выбран** |
| `BAAI/bge-multilingual-gemma2` | 3584 | ~18 GB FP16 | ✅ SOTA на MIRACL / MTEB-pl / MTEB-fr | Gemma TOS (не permissive) | ⚠️ требует не-cpu образ | ❌ R2, R4, R5 |
| `BAAI/bge-en-icl` | 4096 | ~14 GB | ❌ только EN | Apache 2.0 | LLM-style | ❌ R3 |
| `BAAI/bge-large-en-v1.5` | 1024 | ~700 MB | ❌ только EN | MIT | ✅ | ❌ R3 |
| `Qwen3-Embedding-0.6B` | 1024 | ~1.2 GB | ✅ | Apache 2.0 | ✅ | ⚠️ см. ниже |
| `Qwen3-Embedding-8B` | 4096 | ~16 GB | ✅ MTEB-multi top | Apache 2.0 | ✅ | ❌ R2, R5 |
| `nvidia/llama-embed-nemotron-8b` | 4096 | ~16 GB | ✅ MMTEB top | NVIDIA Open Model | ⚠️ | ❌ R2, R4 |
| `google/gemini-embedding-001` | configurable | — | ✅ MTEB top | Cloud API only | — | ❌ R1 |
| `intfloat/multilingual-e5-large` | 1024 | ~1.1 GB | ✅ 100+ langs | MIT | ✅ | ⚠️ см. ниже |

### Близкие альтернативы и почему отвергнуты

**`Qwen3-Embedding-0.6B`** — единственный кандидат, удовлетворяющий всем R1–R7 одновременно. Отвергнут потому, что:
- На MIRACL `bge-m3` (≈70.0 nDCG@10 в комбо-режиме) опережает `Qwen3-Embedding-0.6B`; SOTA-числа Qwen относятся к 8B-варианту, не к 0.6B.
- `bge-m3` поддерживает **dense + sparse + multi-vector retrieval** в одной модели. Сейчас используется только dense, но sparse/multi-vec — запасной путь, если dense окажется недостаточным для Radar (например, для редких терминов). У `Qwen3-Embedding` только dense.
- `bge-m3` уже verified в TEI cpu-1.9; миграция на Qwen потребовала бы перевалидации совместимости и пересчёта всех embeddings.

**`intfloat/multilingual-e5-large` (mE5)** — близкий конкурент: 1024 dim, 100+ языков, MIT, ~1.1 GB. Отвергнут потому, что:
- M3-paper (arXiv 2402.03216) показывает разрыв ~+5 nDCG@10 в пользу `bge-m3` на MIRACL, причём разрыв шире на низкоресурсных языках — важно для R3.
- У mE5 нет sparse / multi-vec режимов.

**`gemini-embedding-001`, `Qwen3-Embedding-8B`, `Llama-Embed-Nemotron-8B`, `bge-multilingual-gemma2`** — топ MTEB Multilingual, но каждый нарушает минимум одно из жёстких требований: либо cloud-only (R1), либо 8–9B параметров (R2, R5), либо restrictive license (R4).

## Известные слабые места `bge-m3`

Перечислены явно, чтобы не было сюрпризов в эксплуатации:

- **Низкоресурсные языки** (баскский, эстонский, многие африканские, мн. малых языков Океании) — качество заметно ниже среднего. Это общее ограничение всех open-weight embedding-моделей такого размера, не специфичное для `bge-m3`. Cross-lingual режим (тема на низкоресурсном языке → статья на en/ru/zh) работает приемлемо благодаря токенизатору XLM-RoBERTa.
- **SOTA-разрыв с большими моделями.** `bge-multilingual-gemma2` / `Qwen3-Embedding-8B` / `Llama-Embed-Nemotron-8B` объективно лучше на бенчмарках, но требуют 16+ GB RAM. Вопрос «extended embedding mode» с большой моделью — потенциальная **дополнительная** конфигурация на будущее, не замена дефолта.
- **Не обучался на коде.** Для технического контента (документация, dev-блоги) семантический поиск может проседать. Текущий scope Radar — новостные RSS-фиды.

## Когда пересматривать решение

Решение не вечное. Триггеры пересмотра:

1. Появление open-weight multilingual-модели с RAM ≤ 2 GB и приростом ≥ 5 nDCG@10 на MIRACL относительно `bge-m3`.
2. Воспроизводимые жалобы пользователей на качество retrieval для конкретных языков (с примерами тем и статей).
3. Прекращение поддержки `bge-m3` в новых релизах TEI (маловероятно — модель массовая).
4. Изменение R1 (например, появление cloud API c гарантиями privacy и opt-in от пользователя). Не отменит R5.

**Стоимость смены модели** при пересмотре: миграция pgvector schema (если меняется dim), background job на пересчёт всех existing embeddings (`radar_topics.embedding`, `radar_findings.embedding`), пересборка HNSW-индексов, обновление `internal/core/embeddings/client_smoke_test.go`.

## Источники

- `BAAI/bge-m3` model card: https://huggingface.co/BAAI/bge-m3
- M3-Embedding paper (ACL 2024, arXiv 2402.03216): https://arxiv.org/abs/2402.03216
- MTEB / MMTEB Leaderboard: https://huggingface.co/spaces/mteb/leaderboard
- MMTEB benchmark paper (arXiv 2502.13595): https://arxiv.org/abs/2502.13595
- HuggingFace TEI: https://github.com/huggingface/text-embeddings-inference
- BGE Series docs: https://bge-model.com/