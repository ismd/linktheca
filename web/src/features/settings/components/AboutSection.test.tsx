import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { AboutSection } from "./AboutSection";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("AboutSection", () => {
  it("shows version, mode, and the radar sweep line when radar is enabled", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: null })),
    );
    render(<AboutSection />, { wrapper: wrapper() });
    expect(screen.getByText("v0.1.0")).toBeInTheDocument();
    expect(screen.getByText("self-hosted")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByText("Awaiting first sweep")).toBeInTheDocument(),
    );
  });

  it("shows Disabled when radar is turned off", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json(
          { code: "radar_disabled", message: "radar feature is disabled on this server" },
          { status: 501 },
        )),
    );
    render(<AboutSection />, { wrapper: wrapper() });
    await waitFor(() =>
      expect(screen.getByText("Disabled")).toBeInTheDocument(),
    );
  });
});
