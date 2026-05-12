// Deterministic mapping id → one of the 10 mock-image gradient classes from globals.css.
// Used while we have no real og:image extraction.

export function gradientClassFor(id: number): string {
  const bucket = ((id - 1) % 10 + 10) % 10; // safe for negative/zero
  return `img-${bucket + 1}`;
}
