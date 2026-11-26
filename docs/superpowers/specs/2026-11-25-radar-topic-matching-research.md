# Radar: why short topics match noise, and what fixes it

**Status:** research, 2026-11-25. Recommendation 1 implemented 2026-11-26 in
a simpler form found while planning it (see "Follow-up" below and
[the plan](../plans/2026-11-26-radar-hub-corrected-matching.md)). The
cross-encoder and LLM-based ideas are deferred.
**Builds on:** [`2026-11-20-radar-full-text-matching-design.md`](2026-11-20-radar-full-text-matching-design.md).

## Trigger

A topic described as just "Steam Deck" previewed five Hacker News findings
at 0.54–0.56: an indie game on Steam, the Marshall Islands stick chart, a
terminal planetarium, a train game, a 4X game VM. All five were above the
0.52 cutoff, none was about the Steam Deck. Users will type two-word topics,
so Radar has to handle them.

## How it was measured

- **Corpus.** 2,232 HN stories: 1,300 sampled from everything that reached
  100 points between 2026-08-28 and 2026-11-19 (the background stream), plus
  932 found by title search for 17 topics, so every topic has something to
  find. Articles were downloaded and embedded exactly as the app does it
  (title + first 2,000 characters, bge-m3 via TEI); 1,715 (77%) have text,
  the rest fall back to the title, close to the 76% in the earlier sample.
- **Topics.** 17 a user might type, in three phrasings each:
  - *short*: what people type ("Steam Deck", "SQLite", "nuclear power",
    "Ozempic");
  - *long*: one sentence, as the dialog asks ("Valve's Steam Deck handheld
    gaming PC: new hardware, SteamOS updates and which games run on it.");
  - *HyDE*: a made-up article opening, what an LLM expansion would produce.
  Four are deliberately ambiguous (Rust, Zig, Starship, Steam). Two broad
  topics ("Large language models", "Computer security") were checked
  separately.
- **Labels.** Pooled TREC-style: every keyword hit, the top 25 for every
  phrasing, and everything any short or long phrasing scores ≥ 0.50; 1,815
  judgments, graded 2 (about the topic, 832), 1 (closely related, 155, left
  out of scoring), 0. One person's labels.
- **Metric.** The product has one default threshold, so each method gets one
  global threshold, picked leave-one-topic-out (LOTO) to avoid fitting it to
  the topic it is scored on. Reported: macro-F1 over the 34 short + long
  topic phrasings ("mixed"), MAP (ranking alone), and false alerts per 1,000
  background findings, which is the "topic with nothing to find" case of the
  screenshot.

**Known bias.** Positives were found by title search, so they nearly always
contain the topic's name in the title. That flatters short phrasings and
anything lexical. False-alert counts come from the background stream and are
not affected.

## What goes wrong

### 1. A two-word topic does not say what it means

The background findings that score highest against "Steam Deck":

| cos | finding |
|---|---|
| 0.62 | Steam Frame starts at $1059 |
| 0.61 | Steam's login method is kinda interesting |
| 0.59 | Introducing Sid Meier's Civilization V for SteamOS |
| 0.59 | StemDeck, a free, open-source and local AI stem separator |
| 0.54 | Steam Users Beg for "No AI" Filter as Vibecoders Flood Next Fest |

The real Steam Deck articles have a 10th percentile of 0.59. Siblings that
share a word ("Steam"), look-alikes that share sub-word tokens ("StemDeck")
and genre neighbours (games, console hacking) land in the same band. With so
little text the topic vector is mostly its tokens. No transformation of the
score separates these, because the ranking itself is wrong; only more
information about the intent does. The same happens the other way round:
for "Ozempic", "The brain may be about to have its Ozempic moment" (0.64)
outscores most real Ozempic articles (median 0.46).

### 2. Every topic has its own noise floor

The mean similarity of a topic to unrelated findings ranges from 0.25 to
0.42 across topics and phrasings; its spread from 0.04 to 0.07. A fixed 0.52
is far above the noise for some topics and inside it for others: per 1,300
background findings it lets through no false alerts for some topics and up
to 20 for others ("remote work"), 17 for the one-sentence "Kubernetes" and
"Linux kernel".

**Short is not the culprit.** On average, short phrasings did *not* sit
higher than long ones: at 0.52 they produced 2.2 false alerts per 1,000
background findings, one-sentence descriptions 6.0. Long descriptions pick up
generic words that pull in unrelated findings: "...and how people use it in
production" made SQLite match Kubernetes war stories, "...the projects people
build with them" made Raspberry Pi match half of Show HN. Fitting separate
thresholds by length confirms it: the best pair is 0.51 for short and 0.54
for long, the opposite of raising the bar for short topics, and it gains
little (F1 0.785 → 0.802).

### 3. Broad topics are a different problem

For "Large language models" and "Computer security" the issue is recall, not
noise: at 0.52 precision is 0.92–0.93 but recall only 0.28–0.29 of the
relevant findings among each topic's top 120.

## What was tried

LOTO macro-F1 over short + long phrasings; FP = false alerts per 1,000
background findings at the chosen threshold.

| Method | F1 | MAP | FP | Notes |
|---|---|---|---|---|
| cos ≥ 0.52 (today) | 0.790 | 0.911 | 4.1 | fixed threshold |
| cos ≥ t | 0.785 | 0.911 | 3.3 | t = 0.525 |
| cos, separate t for short/long | 0.802 | 0.911 | 2.6 | 0.51 / 0.54 |
| z-score against the background, z ≥ t | 0.818 | 0.911 | 2.4 | per-topic mean/sd over recent findings |
| cos ≥ 0.48 and z ≥ 2.7 | 0.824 | 0.911 | 2.1 | absolute floor + relative cut |
| hub correction: cos − finding's mean similarity to 100 unrelated topics | 0.840 | 0.928 | 1.5 | improves ranking itself |
| cos ≥ 0.47 and z(hub-corrected) ≥ 3.1 | 0.851 | 0.928 | 1.9 | |
| mean-centred vectors | 0.833 | 0.911 | 2.1 | |
| title embedding: (cos + title cos) / 2 | 0.833 | — | 1.7 | extra ~15-token embedding per finding |
| title fusion + hub correction: fused ≥ 0.42 and z ≥ 3.5 | 0.890 | — | 1.7 | |
| **title fusion + hub correction, one threshold: fused − hub ≥ 0.175** | **0.897** | **0.962** | **1.4** | added 2026-11-26; best without new models |
| cross-encoder alone, rerank ≥ 0.005 | 0.871 | — | 1.4 | candidates: cos ≥ 0.45, top 80 |
| cross-encoder as a veto on the row above | 0.895 | — | **0.8** | rerank ≥ 0.002 |
| HyDE phrasing (LLM-style article) | 0.754 | 0.867 | 4.2 | HyDE phrasings only; worse than short (0.808) and long (0.790) alone |
| feedback: 3 relevant + 3 irrelevant marked, Rocchio | 0.855 | — | 1.7 | vs 0.807 for the same 6 labels without moving the vector |

The title-fusion rule also makes the phrasing stop mattering: with one
threshold for all of them it scores 0.907 on short phrasings, 0.893 on
sentences, 0.915 on "name (what it is)" and 0.896 on all three mixed. Plain
cosine scores 0.809, 0.782, 0.819 and 0.798.

Lexical evidence was scored on short phrasings only (a sentence has no
phrase to look for): requiring the topic's phrase in the article gives
short-topic F1 0.864, but its recall is 1.0 for named things and 0.47–0.72
for concepts and synonyms ("remote work" vs return-to-office, "Ozempic" vs
GLP-1/Wegovy, NixOS vs Nix, "nuclear power" vs reactors), and it cannot work
across languages, which the embedding model was chosen for (R3). Added to the
normalized score as a bonus it brought nothing (LOTO chose weight 0).

**Phrasing as "name (what it is)".** "Steam Deck (Valve's handheld gaming
PC)", "Zig (programming language)", "Starship (SpaceX rocket)": the best
ranking of all phrasings (MAP 0.938 vs 0.919 short, 0.904 long; F1 0.819 vs
0.809 / 0.782). For Steam Deck it lowers Steam Frame 0.62 → 0.56 and StemDeck
0.59 → 0.52 while real articles stay at 0.60–0.66.

**The quota effect.** Any per-topic z-score lets roughly the same share of
the stream through for every topic (here 0.3–0.5%). That is what removes the
noise of "remote work", and it is also what starves broad topics: recall for
"Large language models" falls from 0.29 to 0.15–0.17 and for "Computer
security" from 0.28 to 0.08–0.10 (title fusion: 0.21 and 0.18 before the
z-cut). Matches per topic are capped by design, so broad topics need their
threshold set by the user or learned from feedback.

**Feedback.** Moving the topic vector toward three findings the user marked
relevant and away from three marked irrelevant (Rocchio, 1.0 / 0.5) lifts F1
from 0.807 to 0.855 for the same six labels; sentences gain most (0.779 →
0.870). Negative marks on their own do not work as a vector shift: the
rejected findings share the topic's genre, so a small weight changes nothing
and a large one drags the topic away from its genre (recall 0.94 → 0.69).

## The empty-stream case

The screenshot's situation: nothing in the stream is about the topic, and
anything shown is noise. Over 17 topics × 1,300 background findings:

| Rule | False alerts | Steam Deck lets through |
|---|---|---|
| cos ≥ 0.52 (today) | 49, of them 20 on "remote work" | Steam Frame, StemDeck, Steam "No AI" filter |
| title fusion + hub-corrected z | 49, 0–6 per topic | the same three + Steam Client |
| + cross-encoder veto, short phrasing | 11 | StemDeck, Steam Frame |
| + cross-encoder veto, "name (what it is)" | **6** | StemDeck |

Calibration evens the noise out: no topic floods any more, but topics that
were silent now get a few alerts, because a z-score always lets the top of
the stream through. Only a model that reads topic and article together
removes most of it. StemDeck survives everything: for "Steam Deck" the
cross-encoder scores it 0.988, as a typo of the query. One click on "not
this" is the only fix for that kind of finding.

## Cross-encoder

`BAAI/bge-reranker-v2-m3` (XLM-RoBERTa, MIT, multilingual like bge-m3) runs
in a second TEI container on `/rerank`.

- **Quality.** On its own it does not beat calibrated dense scores (F1 0.871
  vs 0.890). It wins on named things (Steam Deck, SQLite, Zig, Passkeys) and
  loses on ambiguous or conceptual ones ("Rust" as a sentence: 0.62 vs 0.84;
  "remote work", "Ozempic"). As a veto behind the dense rule it keeps F1
  (0.895) and halves false alerts overall, by 4–8× in the empty-stream case.
- **Its scale depends on the phrasing too.** For "Steam Deck" real articles
  score 0.93–0.99; for the one-sentence description the same articles score
  0.06–0.79. Thresholds sit near 0.002–0.01.
- **Cost.** About 1 s of CPU per pair (22 cores, article cut to 1,200
  characters; 1.5 s with 2,000), comparable to embedding the article. As a
  veto it only sees what the dense rule already accepted, ~0.5% of
  finding × topic pairs. Weights are 2.3 GB fp32; the container held 5 GB
  with `--max-batch-tokens 16384`. Too slow for the live preview: five rows
  take about five seconds.

## Recommendation

### 1. Calibrate the dense score (no new model)

> **Follow-up, 2026-11-26.** Checked again while planning: the per-topic
> z-score is not needed. Title fusion and hub correction with one global
> threshold, `fused − hub ≥ 0.175`, score F1 0.897 vs 0.889, false alerts 1.4
> vs 1.7 per 1,000, 28 vs 49 in the empty stream (short phrasings), and keep
> more of the broad topics (recall 0.17 / 0.21 vs 0.10 / 0.13 for LLMs /
> Security), with no background statistics to maintain. The score is an inner
> product, `(q − c) · (d + t)/2` with `c` the reference centroid, so it is
> indexable. A background sample of 100–1,300 findings changed the z rule by
> under 0.01 F1; title weights 0.4–0.6 and hub weights 0.75–1.25 by under
> 0.005. That is what got implemented, with threshold 0.18. The rest of this
> section is the original recommendation.

- Embed the finding's **title** separately (one ~15-token call, ~0.1 s) and
  score with the mean of article and title similarity.
- Store a **hub score** per finding: its mean similarity to a fixed set of
  ~100 reference topics, computed when it is embedded, and subtract it.
- Keep **per-topic background statistics**: mean and sd of that score over
  the user's recent findings (subscribed feeds, last ~2,000; all feeds while
  there are fewer than a few hundred). Compute them on create and edit, the
  same way the preview does, and refresh daily.
- Match when `fused ≥ 0.42` and `z ≥ 3.5`. `match_threshold` becomes a
  strictness in z units; nobody has tuned theirs yet.

On this corpus: F1 0.785 → 0.890, false alerts 3.3 → 1.7 per 1,000, the
worst topic 20 → 6, and one threshold that holds for any phrasing. The cost
is the quota effect on broad topics, which is the reason the strictness
slider (deferred, see the threshold slider spec) and feedback matter.

### 2. Make the topic say what it means

- Dialog tip: *"Name it, then say what it is: Steam Deck (Valve's handheld
  gaming PC)."* Nudge when the description is one or two words. This is the
  best phrasing measured, and it costs nothing.
- Preview rows get ✓ / ✗ next to the link added on 2026-11-25. ✓ moves the
  topic vector toward the example (Rocchio). ✗ hides that finding and raises
  the topic's strictness just above it rather than moving the vector. The
  same buttons on matches keep it learning. The full-text spec left room for
  this: the description's own vector stays as the base.
- Later, with the opt-in LLM: suggest the "(what it is)" part for short
  descriptions.

### 3. Optional: cross-encoder veto

Off by default, for hosts with ~5 GB to spare: run accepted matches through
the reranker before they reach the inbox. It is the only measured thing that
empties the empty stream. It stays out of the live preview.

### Not recommended

- **A higher threshold for short topics.** On this data the best split goes
  the other way (0.51 short, 0.54 long), gains little (0.785 → 0.802), and
  per-topic calibration covers it.
- **Requiring the topic's words in the article.** Works for names, fails for
  concepts and synonyms (recall 0.47–0.72) and across languages.
- **LLM-written article openings (HyDE) as topic vectors.** Ranked worse
  than both the user's short phrasing and a sentence (MAP 0.867).

## Caveats

- English-only corpus from one source (HN); the multilingual behaviour that
  bge-m3 was picked for, and how the English reference topics behave against
  other languages, is untested.
- One annotator; positives found by title search (see "Known bias").
- Only two broad topics, judged within their top 120.
- The cross-encoder only saw candidates with cos ≥ 0.45 (top 80 per topic),
  and article text cut to 1,200 characters.
- Thresholds are fitted to this corpus; check them on a real instance with
  `radar-sim` before shipping.

## Sources

- Arampatzis et al., score-distributional threshold optimization for
  adaptive filtering (TREC-9/2001 Filtering Track); Arampatzis & Kamps,
  [Score distributions in IR](https://e.humanities.uva.nl/publications/2009/aram_scor09.pdf),
  [signal-to-noise score normalization](https://e.humanities.uva.nl/publications/2009/aram_sign09.pdf).
  Radar is an adaptive filtering system; per-profile thresholds are the
  classic answer.
- Sciavolino et al., [Simple Entity-Centric Questions Challenge Dense
  Retrievers](https://aclanthology.org/2021.emnlp-main.496) (EMNLP 2021):
  dense retrievers fall behind BM25 on rare entities.
- [Dense retrievers and keyphrase queries](https://arxiv.org/html/2412.03193v1)
  (2024): dense models are trained on questions and degrade on keyword-style
  queries.
- Conneau et al. CSLS and [hubness in sentence
  embeddings](https://arxiv.org/abs/2311.18364): correcting for points that
  are close to everything.
- Su et al., [Whitening sentence representations](https://arxiv.org/abs/2103.15316);
  [calibrated similarity](https://arxiv.org/abs/2601.16907): raw cosine ranks
  well but its absolute value means different things in different places.
- Gao et al. HyDE; Wang et al., [Query2doc](https://arxiv.org/abs/2303.07678).
- [bge-reranker-v2-m3](https://huggingface.co/BAAI/bge-reranker-v2-m3);
  [TEI](https://github.com/huggingface/text-embeddings-inference) serves it
  via `/rerank`, but not bge-m3's sparse output (`/embed_sparse` is
  SPLADE-only).
- Feedly's AI Feeds describe the same product move: train on examples, and
  filter by how prominently an article is about the topic, not by keyword
  ([guide](https://docs.feedly.com/article/699-guide-to-ai-feeds-market-intel)).
