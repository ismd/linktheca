import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

  it("shows the downloaded article instead of the summary", async () => {
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({
          ...rawMatch("seen"),
          finding: {
            ...rawMatch("seen").finding,
            content: {
              html: "<p>Full <em>article</em> body</p>", text: "Full article body",
              byline: "Ann Author", site_name: "X", reading_time_seconds: 300,
            },
          },
        })),
    );
    render(<Wrap><MatchReader matchId={42} /></Wrap>);

    const em = await screen.findByText("article");
    expect(em.tagName).toBe("EM");
    expect(screen.getByText("Ann Author")).toBeInTheDocument();
    expect(screen.getByText("5 min read")).toBeInTheDocument();
    expect(screen.queryByText("Summary text")).not.toBeInTheDocument();
  });

  it("shows the article text when there is no HTML", async () => {
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({
          ...rawMatch("seen"),
          finding: {
            ...rawMatch("seen").finding,
            content: {
              html: null, text: "Plain article text", byline: null,
              site_name: null, reading_time_seconds: null,
            },
          },
        })),
    );
    render(<Wrap><MatchReader matchId={42} /></Wrap>);

    expect(await screen.findByText("Plain article text")).toBeInTheDocument();
    expect(screen.queryByText("Summary text")).not.toBeInTheDocument();
    expect(screen.queryByText(/min read/)).not.toBeInTheDocument();
  });

  it("shows the summary when the page was not downloaded", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("seen"))),
    );
    render(<Wrap><MatchReader matchId={42} /></Wrap>);

    expect(await screen.findByText("Summary text")).toBeInTheDocument();
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

  it("marks the match as not about its topic and takes the mark back", async () => {
    let verdict: string | null = null;
    const calls: string[] = [];
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({ ...rawMatch("seen"), verdict })),
      http.put("/api/radar/topics/7/feedback/100", async ({ request }) => {
        verdict = ((await request.json()) as { verdict: string }).verdict;
        calls.push(`PUT ${verdict}`);
        return new HttpResponse(null, { status: 204 });
      }),
      http.delete("/api/radar/topics/7/feedback/100", () => {
        verdict = null;
        calls.push("DELETE");
        return new HttpResponse(null, { status: 204 });
      }),
    );

    const user = userEvent.setup();
    render(<Wrap><MatchReader matchId={42} /></Wrap>);

    expect(await screen.findByText("Is this about Local-first?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /not about it/i }));
    expect(await screen.findByText(/hidden from local-first/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByText("Is this about Local-first?")).toBeInTheDocument();
    expect(calls).toEqual(["PUT irrelevant", "DELETE"]);
  });

  it("shows a ✓ given before", async () => {
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({ ...rawMatch("seen"), verdict: "relevant" })),
    );

    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    expect(await screen.findByText(/marked as about local-first/i)).toBeInTheDocument();
  });
});
