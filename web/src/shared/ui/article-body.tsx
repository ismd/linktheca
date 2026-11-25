import type { ReactNode } from "react";

type Props = {
  /** Article HTML. It is rendered as is, so it must come sanitized from the
   * backend, as both readers' endpoints do. */
  html?: string | null;
  /** Plain text, shown when there is no HTML. */
  text?: string | null;
  /** Shown when there is neither. */
  fallback: ReactNode;
};

/** The body of an extracted article, as the Library and Radar readers show it. */
export function ArticleBody({ html, text, fallback }: Props) {
  if (html) {
    return (
      <div
        className="prose-reader drop-cap"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    );
  }

  if (text) {
    return <div className="prose-reader drop-cap whitespace-pre-wrap">{text}</div>;
  }

  return <>{fallback}</>;
}
