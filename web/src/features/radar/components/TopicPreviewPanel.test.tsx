import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { TopicPreviewPanel } from "./TopicPreviewPanel";

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const DESCRIPTION = "CRDTs and offline-first tooling for user-owned data.";

const rawFinding = (id: number, title: string) => ({
  id, feed_id: 5, feed_title: "Lobsters",
  url: `https://x.example/${id}`, title, summary: null,
  published_at: null, discovered_at: "2026-05-18T09:00:00Z",
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("TopicPreviewPanel", () => {
  it("asks for more text instead of probing a too-short description", async () => {
    render(
      <Wrap>
        <TopicPreviewPanel name="" description="short" />
      </Wrap>,
    );
    // No msw handler is registered: an unhandled request would fail the test.
    expect(await screen.findByText(/write a sentence or two/i)).toBeInTheDocument();
  });

  it("lists scored findings with the cutoff drawn in", async () => {
    server.use(
      http.post("/api/radar/topics/preview", () =>
        HttpResponse.json({
          items: [
            { similarity: 0.813, finding: rawFinding(1, "Local-first software") },
            { similarity: 0.402, finding: rawFinding(2, "Quarterly earnings call") },
          ],
          threshold: 0.55,
        })),
    );

    render(
      <Wrap>
        <TopicPreviewPanel name="Local-first" description={DESCRIPTION} />
      </Wrap>,
    );

    expect(await screen.findByText("Local-first software")).toBeInTheDocument();
    expect(screen.getByText("Quarterly earnings call")).toBeInTheDocument();
    expect(screen.getByText("0.81")).toBeInTheDocument();
    expect(screen.getByText("0.40")).toBeInTheDocument();
    expect(screen.getByText(/threshold 0\.55/i)).toBeInTheDocument();
    expect(screen.getByText(/1 above cutoff/i)).toBeInTheDocument();
  });

  it("sends the draft name and description to the server", async () => {
    let captured: unknown = null;
    server.use(
      http.post("/api/radar/topics/preview", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json({ items: [], threshold: 0.55 });
      }),
    );

    render(
      <Wrap>
        <TopicPreviewPanel name="  Local-first  " description={` ${DESCRIPTION} `} />
      </Wrap>,
    );

    expect(await screen.findByText(/nothing in your subscribed feeds/i)).toBeInTheDocument();
    expect(captured).toEqual({ name: "Local-first", description: DESCRIPTION });
  });

  it("explains an offline embedder", async () => {
    server.use(
      http.post("/api/radar/topics/preview", () =>
        HttpResponse.json(
          { error: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );

    render(
      <Wrap>
        <TopicPreviewPanel name="" description={DESCRIPTION} />
      </Wrap>,
    );

    expect(await screen.findByText(/embedder is offline/i)).toBeInTheDocument();
  });
});
