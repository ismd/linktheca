import { useState } from "react";
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { TopicPreviewPanel } from "./TopicPreviewPanel";
import type { TopicMark } from "../types";

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
        <TopicPreviewPanel description="short" />
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
        <TopicPreviewPanel description={DESCRIPTION} />
      </Wrap>,
    );

    expect(await screen.findByText("Local-first software")).toBeInTheDocument();
    expect(screen.getByText("Quarterly earnings call")).toBeInTheDocument();
    expect(screen.getByText("0.81")).toBeInTheDocument();
    expect(screen.getByText("0.40")).toBeInTheDocument();
    expect(screen.getByText(/threshold 0\.55/i)).toBeInTheDocument();
    expect(screen.getByText(/1 above cutoff/i)).toBeInTheDocument();
  });

  it("opens each previewed article in a new tab", async () => {
    server.use(
      http.post("/api/radar/topics/preview", () =>
        HttpResponse.json({
          items: [{ similarity: 0.813, finding: rawFinding(17, "Local-first software") }],
          threshold: 0.55,
        })),
    );

    render(
      <Wrap>
        <TopicPreviewPanel description={DESCRIPTION} />
      </Wrap>,
    );

    const link = await screen.findByRole("link", { name: /local-first software/i });
    expect(link).toHaveAttribute("href", "/radar/findings/17");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("sends the trimmed draft description to the server", async () => {
    let captured: unknown = null;
    server.use(
      http.post("/api/radar/topics/preview", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json({ items: [], threshold: 0.55 });
      }),
    );

    render(
      <Wrap>
        <TopicPreviewPanel description={` ${DESCRIPTION} `} />
      </Wrap>,
    );

    expect(await screen.findByText(/nothing in your subscribed feeds/i)).toBeInTheDocument();
    expect(captured).toEqual({ description: DESCRIPTION });
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
        <TopicPreviewPanel description={DESCRIPTION} />
      </Wrap>,
    );

    expect(await screen.findByText(/embedder is offline/i)).toBeInTheDocument();
  });

  it("has no mark buttons when nothing listens for marks", async () => {
    server.use(
      http.post("/api/radar/topics/preview", () =>
        HttpResponse.json({
          items: [{ similarity: 0.3, finding: rawFinding(1, "Local-first software") }],
          threshold: 0.18,
        })),
    );

    render(
      <Wrap>
        <TopicPreviewPanel description={DESCRIPTION} />
      </Wrap>,
    );

    expect(await screen.findByText("Local-first software")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /about this topic/i })).not.toBeInTheDocument();
  });

  it("marks rows, sends the marks with the next preview and lets them be undone", async () => {
    const sent: unknown[] = [];
    server.use(
      http.post("/api/radar/topics/preview", async ({ request }) => {
        const body = (await request.json()) as { feedback?: unknown };
        sent.push(body.feedback ?? null);
        return HttpResponse.json({
          items: [
            { similarity: 0.31, finding: rawFinding(1, "Local-first software") },
            { similarity: 0.26, finding: rawFinding(2, "When Ldaxr doesn't work") },
          ],
          threshold: 0.18,
        });
      }),
    );

    function Marking() {
      const [marks, setMarks] = useState<TopicMark[]>([]);
      return <TopicPreviewPanel description={DESCRIPTION} marks={marks} onMarksChange={setMarks} />;
    }

    const user = userEvent.setup();
    render(
      <Wrap>
        <Marking />
      </Wrap>,
    );

    const good = (await screen.findByText("Local-first software")).closest("li")!;
    await user.click(within(good).getByRole("button", { name: "About this topic" }));
    expect(within(good).getByRole("button", { name: "About this topic" }))
      .toHaveAttribute("aria-pressed", "true");

    const noise = screen.getByText("When Ldaxr doesn't work").closest("li")!;
    await user.click(within(noise).getByRole("button", { name: "Not about this topic" }));
    expect(screen.queryByText("When Ldaxr doesn't work")).not.toBeInTheDocument();
    expect(screen.getByText("Marked 1 ✓ · 1 ✗")).toBeInTheDocument();

    await waitFor(() =>
      expect(sent.at(-1)).toEqual([
        { finding_id: 1, verdict: "relevant" },
        { finding_id: 2, verdict: "irrelevant" },
      ]));

    await user.click(screen.getByRole("button", { name: "Show" }));
    const hidden = screen.getByText("When Ldaxr doesn't work").closest("li")!;
    await user.click(within(hidden).getByRole("button", { name: "Undo" }));
    expect(screen.getByText("Marked 1 ✓ · 0 ✗")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Forget all" }));
    expect(screen.queryByText(/^Marked/)).not.toBeInTheDocument();
    await waitFor(() => expect(sent.at(-1)).toBeNull());
  });
});
