import { useEffect, useRef } from "react";
import { useAuthStore } from "./store";
import { readRefreshToken } from "./storage";
import { me } from "./api";

export function useBootstrap(): void {
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;

    const refreshToken = readRefreshToken();
    if (!refreshToken) {
      useAuthStore.getState().clearSession();
      return;
    }

    (async () => {
      try {
        await me(); // апи сам зовёт /auth/refresh при 401 и обновит store
      } catch {
        useAuthStore.getState().clearSession();
      }
    })();
  }, []);
}
