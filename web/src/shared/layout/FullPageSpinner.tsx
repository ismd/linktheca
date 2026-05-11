export function FullPageSpinner() {
  return (
    <div
      role="status"
      aria-live="polite"
      className="paper-surface fixed inset-0 flex flex-col items-center justify-center gap-6"
    >
      <p className="font-display italic text-4xl text-ink leading-none">Linktheca</p>
      <div className="flex gap-2">
        <span className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse" />
        <span
          className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse"
          style={{ animationDelay: "120ms" }}
        />
        <span
          className="block h-1.5 w-1.5 rounded-full bg-ink-3 animate-pulse"
          style={{ animationDelay: "240ms" }}
        />
      </div>
      <span className="sr-only">Loading</span>
    </div>
  );
}
