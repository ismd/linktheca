export const TOPIC_DESCRIPTION_PLACEHOLDER =
  "New releases and benchmarks of open-weight LLMs: Llama, Qwen, Mistral, DeepSeek";

/**
 * How to phrase a topic so the embedding finds what the user means. Radar
 * compares the description with each article's text, so it should read like
 * the article itself rather than like a search request.
 */
export function TopicWritingTips({ id }: { id: string }) {
  return (
    <ul id={id} className="font-body text-sm text-muted-foreground flex flex-col gap-0.5">
      <li>Write it like the headline of an article you&rsquo;d want to read.</li>
      <li>Name concrete things: products, projects, people, places.</li>
      <li>
        One subject per topic. &ldquo;Not about X&rdquo; won&rsquo;t filter X out: mark such
        articles &#x2717; instead.
      </li>
    </ul>
  );
}
