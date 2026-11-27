import { useState } from "react";
import { Check, X } from "lucide-react";
import { ApiError } from "@/shared/api/errors";
import { useDebouncedValue } from "@/shared/lib/use-debounced-value";
import {
  MIN_PREVIEW_DESCRIPTION,
  useTopicPreviewQuery,
} from "../use-radar";
import type { MatchFinding, PreviewMatch, TopicMark, Verdict } from "../types";

const DEBOUNCE_MS = 500;

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError && err.code === "embedder_unavailable") {
    return "Embedder is offline — no preview right now.";
  }
  return "Could not score this description.";
}

/** marks with the verdict on finding set, or taken back when verdict is null. */
function withMark(marks: TopicMark[], finding: MatchFinding, verdict: Verdict | null): TopicMark[] {
  const rest = marks.filter((m) => m.finding.id !== finding.id);
  return verdict === null ? rest : [...rest, { verdict, finding }];
}

function FindingLink({ finding: f, muted }: { finding: MatchFinding; muted: boolean }) {
  return (
    <div className="min-w-0 flex-1">
      {/* A new tab keeps the half-written topic in this dialog intact. */}
      <a
        href={`/radar/findings/${f.id}`}
        target="_blank"
        rel="noopener noreferrer"
        className={`block font-body text-sm leading-snug truncate underline-offset-2 hover:text-vermillion hover:underline ${
          muted ? "text-muted-foreground" : "text-ink"
        }`}
      >
        {f.title ?? host(f.url)}
        <span className="sr-only"> (opens in a new tab)</span>
      </a>
      <p className="label-sc text-muted-foreground">
        {f.feedTitle ?? host(f.url)}
      </p>
    </div>
  );
}

const markButton =
  "inline-flex h-7 w-7 items-center justify-center border transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ink/20";

function MarkButtons({
  finding,
  verdict,
  onMark,
}: {
  finding: MatchFinding;
  verdict: Verdict | null;
  onMark: (finding: MatchFinding, verdict: Verdict | null) => void;
}) {
  const relevant = verdict === "relevant";
  return (
    <div className="flex shrink-0 gap-1 self-center">
      <button
        type="button"
        aria-label="About this topic"
        aria-pressed={relevant}
        title="About this topic"
        onClick={() => onMark(finding, relevant ? null : "relevant")}
        className={`${markButton} ${
          relevant
            ? "border-ink bg-ink text-paper"
            : "border-rule text-muted-foreground hover:border-ink hover:text-ink"
        }`}
      >
        <Check className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
      <button
        type="button"
        aria-label="Not about this topic"
        title="Not about this topic"
        onClick={() => onMark(finding, "irrelevant")}
        className={`${markButton} border-rule text-muted-foreground hover:border-vermillion hover:text-vermillion`}
      >
        <X className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </div>
  );
}

function PreviewRow({
  item,
  muted,
  verdict,
  onMark,
}: {
  item: PreviewMatch;
  muted: boolean;
  verdict: Verdict | null;
  onMark?: (finding: MatchFinding, verdict: Verdict | null) => void;
}) {
  return (
    <li className="flex items-baseline gap-3 py-2">
      <span
        className={`font-mono text-xs tabular-nums shrink-0 ${
          muted ? "text-muted-foreground" : "text-vermillion"
        }`}
      >
        {item.similarity.toFixed(2)}
      </span>
      <FindingLink finding={item.finding} muted={muted} />
      {onMark && <MarkButtons finding={item.finding} verdict={verdict} onMark={onMark} />}
    </li>
  );
}

function ThresholdRule({ threshold }: { threshold: number }) {
  return (
    <li className="flex items-center gap-3 py-2" aria-hidden="true">
      <span className="h-px flex-1 bg-rule" />
      <span className="label-sc text-muted-foreground">
        threshold {threshold.toFixed(2)}
      </span>
      <span className="h-px flex-1 bg-rule" />
    </li>
  );
}

function PreviewSkeleton() {
  return (
    <ul className="flex flex-col">
      {[0, 1, 2].map((i) => (
        <li key={i} className="flex items-center gap-3 py-2">
          <span className="skeleton h-3 w-8 shrink-0" />
          <span className="skeleton h-3 flex-1" />
        </li>
      ))}
    </ul>
  );
}

/** The marks given so far, each of which can be taken back. */
function MarksSummary({
  marks,
  onChange,
}: {
  marks: TopicMark[];
  onChange: (marks: TopicMark[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const relevant = marks.filter((m) => m.verdict === "relevant").length;
  const irrelevant = marks.length - relevant;

  return (
    <div className="mt-1 border-t border-rule pt-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="label-sc text-ink-3">
          Marked {relevant} ✓ · {irrelevant} ✗
        </span>
        <button
          type="button"
          className="label-sc text-muted-foreground underline-offset-2 hover:text-ink hover:underline"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          {open ? "Hide" : "Show"}
        </button>
        <button
          type="button"
          className="label-sc text-muted-foreground underline-offset-2 hover:text-vermillion hover:underline"
          onClick={() => onChange([])}
        >
          Forget all
        </button>
      </div>
      {open && (
        <ul className="flex flex-col">
          {marks.map((m) => (
            <li key={m.finding.id} className="flex items-baseline gap-3 py-1.5">
              <span
                className={`font-mono text-xs shrink-0 ${
                  m.verdict === "relevant" ? "text-ink" : "text-vermillion"
                }`}
                aria-label={m.verdict === "relevant" ? "About this topic" : "Not about this topic"}
              >
                {m.verdict === "relevant" ? "✓" : "✗"}
              </span>
              <FindingLink finding={m.finding} muted={m.verdict === "irrelevant"} />
              <button
                type="button"
                className="label-sc shrink-0 text-muted-foreground underline-offset-2 hover:text-ink hover:underline"
                onClick={() => onChange(withMark(marks, m.finding, null))}
              >
                Undo
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

type Props = {
  description: string;
  /** Marks given so far. With onMarksChange the rows can be marked ✓ / ✗. */
  marks?: TopicMark[];
  onMarksChange?: (marks: TopicMark[]) => void;
};

export function TopicPreviewPanel({ description, marks = [], onMarksChange }: Props) {
  const debouncedDescription = useDebouncedValue(description, DEBOUNCE_MS);
  const query = useTopicPreviewQuery(debouncedDescription, marks);

  const tooShort = debouncedDescription.trim().length < MIN_PREVIEW_DESCRIPTION;
  const verdicts = new Map(marks.map((m) => [m.finding.id, m.verdict]));
  const preview = query.data;
  // A row marked ✗ leaves at once, before the preview is scored again.
  const items = (preview?.items ?? []).filter((i) => verdicts.get(i.finding.id) !== "irrelevant");
  const aboveCount = preview
    ? items.filter((i) => i.similarity >= preview.threshold).length
    : 0;

  const onMark = onMarksChange
    ? (finding: MatchFinding, verdict: Verdict | null) =>
        onMarksChange(withMark(marks, finding, verdict))
    : undefined;

  return (
    <section
      className="border border-rule bg-paper-2 px-3 py-2"
      aria-live="polite"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="label-sc text-ink-3">Would match</h3>
        {!tooShort && query.isFetching && (
          <span className="label-sc text-muted-foreground">scoring…</span>
        )}
        {!tooShort && !query.isFetching && preview && items.length > 0 && (
          <span className="label-sc text-muted-foreground">
            {aboveCount} above cutoff
          </span>
        )}
      </div>
      {onMark && !tooShort && items.length > 0 && (
        <p className="font-body text-xs text-muted-foreground">
          Mark rows ✓ or ✗ and the topic learns what you mean.
        </p>
      )}

      {tooShort ? (
        <p className="font-body text-sm text-muted-foreground py-2">
          Write a sentence or two and Radar will show what it would catch.
        </p>
      ) : query.isError ? (
        <p className="font-body text-sm text-vermillion-dark py-2">
          {errorMessage(query.error)}
        </p>
      ) : !preview ? (
        <PreviewSkeleton />
      ) : items.length === 0 ? (
        <p className="font-body text-sm text-muted-foreground py-2">
          Nothing in your subscribed feeds scores against this yet.
        </p>
      ) : (
        <ul className="flex flex-col">
          {items.map((item, i) => (
            <PreviewRowWithCutoff
              key={item.finding.id}
              item={item}
              previous={items[i - 1]}
              threshold={preview.threshold}
              verdict={verdicts.get(item.finding.id) ?? null}
              onMark={onMark}
            />
          ))}
        </ul>
      )}

      {onMarksChange && marks.length > 0 && (
        <MarksSummary marks={marks} onChange={onMarksChange} />
      )}
    </section>
  );
}

/**
 * Renders one row, prefixed by the cutoff rule when this is the first item
 * that falls below the threshold — the same visual radar-sim prints.
 */
function PreviewRowWithCutoff({
  item,
  previous,
  threshold,
  verdict,
  onMark,
}: {
  item: PreviewMatch;
  previous: PreviewMatch | undefined;
  threshold: number;
  verdict: Verdict | null;
  onMark?: (finding: MatchFinding, verdict: Verdict | null) => void;
}) {
  const below = item.similarity < threshold;
  const firstBelow = below && (previous === undefined || previous.similarity >= threshold);

  return (
    <>
      {firstBelow && <ThresholdRule threshold={threshold} />}
      <PreviewRow item={item} muted={below} verdict={verdict} onMark={onMark} />
    </>
  );
}
