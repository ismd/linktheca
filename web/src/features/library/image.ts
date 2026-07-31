// Deterministic mapping id → one of the 10 gradient classes from globals.css.
// Used as the backdrop behind every preview, and on its own for articles that
// have no downloaded image.

export function gradientClassFor(id: number): string {
  const bucket = ((id - 1) % 10 + 10) % 10; // safe for negative/zero
  return `img-${bucket + 1}`;
}

// Downloaded assets live outside the /api prefix: nginx serves them straight
// from the shared volume in production, Vite proxies them to the backend in dev.
const MEDIA_BASE = "/media";

export function previewImageUrl(filename: string): string {
  return `${MEDIA_BASE}/images/${encodeURIComponent(filename)}`;
}

export function faviconUrl(filename: string): string {
  return `${MEDIA_BASE}/favicons/${encodeURIComponent(filename)}`;
}
