import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "./store";

describe("useAuthStore", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession();
    useAuthStore.setState({ status: "bootstrapping" });
  });

  it("starts in bootstrapping status with no token or user", () => {
    const s = useAuthStore.getState();
    expect(s.status).toBe("bootstrapping");
    expect(s.accessToken).toBeNull();
    expect(s.user).toBeNull();
  });

  it("setSession transitions to authed and stores token+user", () => {
    useAuthStore.getState().setSession("tok-123", {
      id: 1,
      email: "a@b.c",
      displayName: "A",
      isAdmin: false,
    });
    const s = useAuthStore.getState();
    expect(s.status).toBe("authed");
    expect(s.accessToken).toBe("tok-123");
    expect(s.user?.email).toBe("a@b.c");
  });

  it("clearSession resets to anonymous", () => {
    useAuthStore.getState().setSession("t", { id: 1, email: "x", displayName: "X", isAdmin: false });
    useAuthStore.getState().clearSession();
    const s = useAuthStore.getState();
    expect(s.status).toBe("anonymous");
    expect(s.accessToken).toBeNull();
    expect(s.user).toBeNull();
  });
});
