import { useEffect, useRef } from "react";
import { refreshSession } from "@/shared/api/client";
import { useAuthStore } from "./store";
import { readRefreshToken } from "./storage";

export function useBootstrap(): void {
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;

    if (!readRefreshToken()) {
      useAuthStore.getState().clearSession();
      return;
    }

    refreshSession().catch(() => {});
  }, []);
}
