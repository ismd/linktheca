import { create } from "zustand";

export type User = {
  id: number;
  email: string;
  displayName: string;
  isAdmin: boolean;
};

export type AuthStatus = "bootstrapping" | "authed" | "anonymous";

type AuthState = {
  accessToken: string | null;
  user: User | null;
  status: AuthStatus;
  setSession: (accessToken: string, user: User) => void;
  clearSession: () => void;
};

export const useAuthStore = create<AuthState>((set) => ({
  accessToken: null,
  user: null,
  status: "bootstrapping",
  setSession: (accessToken, user) => set({ accessToken, user, status: "authed" }),
  clearSession: () => set({ accessToken: null, user: null, status: "anonymous" }),
}));
