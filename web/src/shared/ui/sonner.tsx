import { Toaster as SonnerToaster } from "sonner";

// Sonner paints the toast from CSS custom properties, and its own stylesheet is
// injected into <head> outside any cascade layer — so Tailwind utilities on the
// toast never win. Setting the variables inline on the toaster is what actually
// takes effect; the rest of the skin lives in styles/globals.css.
const THEME = {
  "--normal-bg": "var(--color-paper-2)",
  "--normal-border": "var(--color-rule)",
  "--normal-text": "var(--color-ink)",
  "--border-radius": "0px",
} as React.CSSProperties;

export function Toaster() {
  return (
    <SonnerToaster
      // Bottom, because the top right corner belongs to the topbar: a toast
      // there covers Add link and blocks saving a second article back to back.
      position="bottom-right"
      richColors={false}
      style={THEME}
    />
  );
}
