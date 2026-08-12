import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { MatchReader } from "./MatchReader";

const rawMatch = (state: "new" | "seen") => ({
  id: 42, topic_id: 7, topic_name: "Local-first",
  similarity: 0.7, state, matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 100, feed_id: 5, feed_title: "Ink & Switch",
    url: "https://x.example/a", title: "Title", summary: "Summary text",
    published_at: "2026-05-17T10:00:00Z",
    discovered_at: "2026-05-18T09:00:00Z",
  },
});

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("MatchReader", () => {
  it("auto-marks state=new as seen on mount (PATCH called exactly once)", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("new"))),
    );
    let patchCount = 0;
    server.use(
      http.patch("/api/radar/matches/42", () => {
        patchCount += 1;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    await screen.findByText("Title");
    await waitFor(() => expect(patchCount).toBe(1));
  });

  it("does NOT mark already-seen match", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("seen"))),
    );
    let patched = false;
    server.use(
      http.patch("/api/radar/matches/42", () => {
        patched = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    await screen.findByText("Title");
    // give effects time to run
    await new Promise((r) => setTimeout(r, 50));
    expect(patched).toBe(false);
  });

  it("falls back when summary is empty", async () => {
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({ ...rawMatch("seen"), finding: { ...rawMatch("seen").finding, summary: null } })),
    );
    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    expect(await screen.findByText(/no summary captured/i)).toBeInTheDocument();
  });

  it("links back to the topic archive under /radar/topics", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("seen"))),
    );
    render(
      <Wrap>
        <MatchReader matchId={42} />
      </Wrap>,
    );
    const links = await screen.findAllByRole("link");
    const topicLinks = links.filter(
      (l) => l.getAttribute("href") === "/radar/topics/7",
    );
    expect(topicLinks).toHaveLength(2);
  });
});
