import { ApiError } from "@/shared/api/errors";
import { useDebouncedValue } from "@/shared/lib/use-debounced-value";
import {
  MIN_PREVIEW_DESCRIPTION,
  useTopicPreviewQuery,
} from "../use-radar";
import type { PreviewMatch } from "../types";

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

function PreviewRow({ item, muted }: { item: PreviewMatch; muted: boolean }) {
  const f = item.finding;
  return (
    <li className="flex items-baseline gap-3 py-2">
      <span
        className={`font-mono text-xs tabular-nums shrink-0 ${
          muted ? "text-muted-foreground" : "text-vermillion"
        }`}
      >
        {item.similarity.toFixed(2)}
      </span>
      <div className="min-w-0">
        <p
          className={`font-body text-sm leading-snug truncate ${
            muted ? "text-muted-foreground" : "text-ink"
          }`}
        >
          {f.title ?? host(f.url)}
        </p>
        <p className="label-sc text-muted-foreground">
          {f.feedTitle ?? host(f.url)}
        </p>
      </div>
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

type Props = {
  name: string;
  description: string;
};

export function TopicPreviewPanel({ name, description }: Props) {
  const debouncedName = useDebouncedValue(name, DEBOUNCE_MS);
  const debouncedDescription = useDebouncedValue(description, DEBOUNCE_MS);
  const query = useTopicPreviewQuery(debouncedName, debouncedDescription);

  const tooShort = debouncedDescription.trim().length < MIN_PREVIEW_DESCRIPTION;
  const preview = query.data;
  const aboveCount = preview
    ? preview.items.filter((i) => i.similarity >= preview.threshold).length
    : 0;

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
        {!tooShort && !query.isFetching && preview && preview.items.length > 0 && (
          <span className="label-sc text-muted-foreground">
            {aboveCount} above cutoff
          </span>
        )}
      </div>

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
      ) : preview.items.length === 0 ? (
        <p className="font-body text-sm text-muted-foreground py-2">
          Nothing in your subscribed feeds scores against this yet.
        </p>
      ) : (
        <ul className="flex flex-col">
          {preview.items.map((item, i) => (
            <PreviewRowWithCutoff
              key={item.finding.id}
              item={item}
              previous={preview.items[i - 1]}
              threshold={preview.threshold}
            />
          ))}
        </ul>
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
}: {
  item: PreviewMatch;
  previous: PreviewMatch | undefined;
  threshold: number;
}) {
  const below = item.similarity < threshold;
  const firstBelow = below && (previous === undefined || previous.similarity >= threshold);

  return (
    <>
      {firstBelow && <ThresholdRule threshold={threshold} />}
      <PreviewRow item={item} muted={below} />
    </>
  );
}
