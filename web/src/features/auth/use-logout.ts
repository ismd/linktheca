import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { logout as logoutApi } from "./api";
import { clearRefreshToken } from "./storage";
import { useAuthStore } from "./store";

export function useLogout(): () => Promise<void> {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  return async () => {
    try {
      await logoutApi();
    } catch {
      // best-effort — locally we still clear
    }
    clearRefreshToken();
    useAuthStore.getState().clearSession();
    queryClient.clear();
    toast.success("Signed out");
    navigate("/login", { replace: true });
  };
}
