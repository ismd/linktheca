# Decision: the embedding model is `BAAI/bge-m3`

**Date:** 2026-05-06
**Status:** approved
**Refines:**
- `2026-04-10-architecture-design.md` — section 6, "Embeddings"
- `2026-04-22-phase-3a-radar-pipeline-design.md` — the "Context" section, item 1

## TL;DR

Linktheca uses **`BAAI/bge-m3`** (1024 dim, MIT) on a **HuggingFace Text Embeddings Inference (TEI)** server for every embedding in the project (Radar topics, Radar findings). The model was picked as the Pareto optimum under hard constraints: self-hosted, a RAM budget, multilingual with cross-lingual retrieval, a permissive licence, dim = 1024 (already fixed in the pgvector schema), and native support in TEI.

The alternatives (`bge-multilingual-gemma2`, `Qwen3-Embedding-*`, `multilingual-e5-large`, `Llama-Embed-Nemotron`, `gemini-embedding-001` and others) were considered and rejected — see the table below.

## Context

Linktheca is an open-source, on-premise service; users deploy it on their own machines and read content in many languages. The Radar module performs semantic retrieval: a user's topic (in any language) looks for articles from RSS in any language. The quality of cross-lingual retrieval and the breadth of language coverage decide how useful Radar is to an international audience.

Embedding dimensionality is a **long-term architectural decision**: switching to a model with a different dim would require migrating the pgvector schema, recomputing every existing embedding, and rebuilding the HNSW indexes. The cost of getting this wrong is high, so the decision gets its own document.

## Hard constraints

| # | Requirement | Source |
|---|---|---|
| **R1** | Self-hosted, no cloud API for embeddings | `CLAUDE.md`; the "embeddings local, LLM opt-in cloud" decision is recorded in the `project_ai_stack_decision.md` memory |
| **R2** | RAM ≤ ~3 GB for the embedding service | Target hardware: a user's home server or VPS, not a data centre |
| **R3** | Multilingual plus **cross-lingual retrieval** (a topic in one language finds articles in others) | The international audience of an open-source project; without cross-lingual retrieval, topic monitoring collapses into monolingual search |
| **R4** | A permissive licence (MIT/Apache 2.0) | Users deploy on their own machines — we cannot bind them to a vendor's restrictive TOS |
| **R5** | dim = 1024 | Already fixed in the pgvector schema (`vector(1024)` in the `006_radar_topics` and `009_radar_findings` migrations) and in the HNSW indexes |
| **R6** | Context ≥ 4K tokens | An article's title + description + summary can run long, especially in languages with long word forms (Finnish, German) |
| **R7** | Compatible with the TEI `cpu-1.9` image | Already wired into `compose.dev.yaml` and exercised by `internal/core/embeddings/client_smoke_test.go` |

## Candidate comparison

| Model | Dim | RAM | Multilingual | License | TEI | Verdict |
|---|---|---|---|---|---|---|
| **`BAAI/bge-m3`** | **1024** | ~2.5 GB | ✅ 100+ langs, MIRACL nDCG@10 ≈ 70.0 (combined mode) | **MIT** | ✅ XLM-RoBERTa native | ✅ **chosen** |
| `BAAI/bge-multilingual-gemma2` | 3584 | ~18 GB FP16 | ✅ SOTA on MIRACL / MTEB-pl / MTEB-fr | Gemma TOS (not permissive) | ⚠️ needs a non-cpu image | ❌ R2, R4, R5 |
| `BAAI/bge-en-icl` | 4096 | ~14 GB | ❌ EN only | Apache 2.0 | LLM-style | ❌ R3 |
| `BAAI/bge-large-en-v1.5` | 1024 | ~700 MB | ❌ EN only | MIT | ✅ | ❌ R3 |
| `Qwen3-Embedding-0.6B` | 1024 | ~1.2 GB | ✅ | Apache 2.0 | ✅ | ⚠️ see below |
| `Qwen3-Embedding-8B` | 4096 | ~16 GB | ✅ MTEB-multi top | Apache 2.0 | ✅ | ❌ R2, R5 |
| `nvidia/llama-embed-nemotron-8b` | 4096 | ~16 GB | ✅ MMTEB top | NVIDIA Open Model | ⚠️ | ❌ R2, R4 |
| `google/gemini-embedding-001` | configurable | — | ✅ MTEB top | Cloud API only | — | ❌ R1 |
| `intfloat/multilingual-e5-large` | 1024 | ~1.1 GB | ✅ 100+ langs | MIT | ✅ | ⚠️ see below |

### The near misses and why they were rejected

**`Qwen3-Embedding-0.6B`** is the only other candidate that satisfies R1–R7 all at once. Rejected because:
- On MIRACL, `bge-m3` (≈70.0 nDCG@10 in combined mode) leads `Qwen3-Embedding-0.6B`; Qwen's SOTA numbers belong to the 8B variant, not to 0.6B.
- `bge-m3` supports **dense + sparse + multi-vector retrieval** in one model. Only dense is used today, but sparse and multi-vector are a fallback if dense turns out to be insufficient for Radar (for rare terms, say). `Qwen3-Embedding` is dense only.
- `bge-m3` is already verified against TEI cpu-1.9; moving to Qwen would mean revalidating compatibility and recomputing every embedding.

**`intfloat/multilingual-e5-large` (mE5)** is a close competitor: 1024 dim, 100+ languages, MIT, ~1.1 GB. Rejected because:
- The M3 paper (arXiv 2402.03216) shows a gap of about +5 nDCG@10 in favour of `bge-m3` on MIRACL, and the gap widens on low-resource languages — which matters for R3.
- mE5 has no sparse or multi-vector modes.

**`gemini-embedding-001`, `Qwen3-Embedding-8B`, `Llama-Embed-Nemotron-8B`, `bge-multilingual-gemma2`** top the MTEB Multilingual board, but each violates at least one hard requirement: cloud-only (R1), 8–9B parameters (R2, R5), or a restrictive licence (R4).

## Known weaknesses of `bge-m3`

Listed explicitly so nothing is a surprise in production:

- **Low-resource languages** (Basque, Estonian, many African languages, many small languages of Oceania) come out noticeably below average. This is a general limitation of every open-weight embedding model at this size, not something specific to `bge-m3`. Cross-lingual mode (a topic in a low-resource language finding an article in en/ru/zh) works acceptably thanks to the XLM-RoBERTa tokenizer.
- **The SOTA gap against larger models.** `bge-multilingual-gemma2` / `Qwen3-Embedding-8B` / `Llama-Embed-Nemotron-8B` are objectively better on benchmarks, but need 16+ GB of RAM. An "extended embedding mode" with a large model is a possible **additional** configuration for the future, not a replacement for the default.
- **It was not trained on code.** For technical content (documentation, dev blogs) semantic search may sag. Radar's current scope is news RSS feeds.

## When to revisit

This decision is not permanent. Triggers for revisiting it:

1. An open-weight multilingual model appears with RAM ≤ 2 GB and a gain of ≥ 5 nDCG@10 on MIRACL over `bge-m3`.
2. Reproducible user complaints about retrieval quality for specific languages (with example topics and articles).
3. `bge-m3` support is dropped in new TEI releases (unlikely — the model is widely used).
4. R1 changes (a cloud API with privacy guarantees and user opt-in, for instance). That would not lift R5.

**The cost of switching models** on a revisit: migrating the pgvector schema (if the dim changes), a background job recomputing every existing embedding (`radar_topics.embedding`, `radar_findings.embedding`), rebuilding the HNSW indexes, and updating `internal/core/embeddings/client_smoke_test.go`.

## Sources

- `BAAI/bge-m3` model card: https://huggingface.co/BAAI/bge-m3
- M3-Embedding paper (ACL 2024, arXiv 2402.03216): https://arxiv.org/abs/2402.03216
- MTEB / MMTEB Leaderboard: https://huggingface.co/spaces/mteb/leaderboard
- MMTEB benchmark paper (arXiv 2502.13595): https://arxiv.org/abs/2502.13595
- HuggingFace TEI: https://github.com/huggingface/text-embeddings-inference
- BGE Series docs: https://bge-model.com/
