import { useEffect, useRef } from "react";

const THRESHOLD = 0.9;

type Args = {
  enabled: boolean;
  onReach: () => void;
};

export function useMarkReadOnScroll({ enabled, onReach }: Args): void {
  const fired = useRef(false);
  const onReachRef = useRef(onReach);

  useEffect(() => {
    onReachRef.current = onReach;
  }, [onReach]);

  useEffect(() => {
    if (!enabled) return;
    fired.current = false;

    function handler() {
      if (fired.current) return;
      const doc = document.documentElement;
      const max = doc.scrollHeight - doc.clientHeight;
      if (max <= 0) return;
      const ratio = doc.scrollTop / max;
      if (ratio >= THRESHOLD) {
        fired.current = true;
        onReachRef.current();
      }
    }

    window.addEventListener("scroll", handler, { passive: true });
    handler(); // run once in case already scrolled past
    return () => window.removeEventListener("scroll", handler);
  }, [enabled]);
}
