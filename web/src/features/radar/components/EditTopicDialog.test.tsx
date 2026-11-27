import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { EditTopicDialog } from "./EditTopicDialog";
import type { TopicWithStats } from "../types";

const topic: TopicWithStats = {
  id: 7, userId: 1, name: "Old name", description: "Old description, long enough.",
  matchThreshold: 0.55, matchCutoff: 0.55, isActive: true, hasEmbedding: true,
  createdAt: new Date(), updatedAt: new Date(),
  stats: {
    newCount: 0, totalCount: 0, sourceCount: 0, lastMatchAt: null,
    markedRelevant: 0, markedIrrelevant: 0,
  },
};

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}

const rawFinding = (id: number, title: string) => ({
  id, feed_id: 5, feed_title: "HN", url: `https://x.example/${id}`, title,
  summary: null, published_at: null, discovered_at: "2026-05-18T09:00:00Z",
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
  // The dialog previews the description and loads the topic's marks.
  server.use(
    http.post("/api/radar/topics/preview", () =>
      HttpResponse.json({ items: [], threshold: 0.18 })),
    http.get("/api/radar/topics/7/feedback", () => HttpResponse.json({ items: [] })),
  );
});

describe("EditTopicDialog", () => {
  it("populates initial values and PATCHes only changed fields", async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.patch("/api/radar/topics/7", async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          id: 7, user_id: 1, name: "Old name", description: "New description, long enough.",
          match_threshold: 0.55, is_active: true, has_embedding: true,
          created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
          stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
        });
      }),
    );

    const user = userEvent.setup();
    const onClose = () => {};
    render(
      <Wrap>
        <EditTopicDialog topic={topic} open={true} onOpenChange={onClose} />
      </Wrap>,
    );

    const descField = screen.getByLabelText(/what to watch for/i) as HTMLTextAreaElement;
    expect(descField.value).toBe("Old description, long enough.");
    await user.clear(descField);
    await user.type(descField, "New description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({ description: "New description, long enough." });
  });

  it("renames without touching the description", async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.patch("/api/radar/topics/7", async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          id: 7, user_id: 1, name: "Fresh name", description: "Old description, long enough.",
          match_threshold: 0.55, is_active: true, has_embedding: true,
          created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
          stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
        });
      }),
    );

    const user = userEvent.setup();
    render(
      <Wrap>
        <EditTopicDialog topic={topic} open={true} onOpenChange={() => {}} />
      </Wrap>,
    );

    const nameField = screen.getByLabelText(/^name$/i);
    await user.clear(nameField);
    await user.type(nameField, "  Fresh name ");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({ name: "Fresh name" });
  });

  it("shows specific error on 503 embedder and stays open", async () => {
    server.use(
      http.patch("/api/radar/topics/7", () =>
        HttpResponse.json(
          { error: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );

    let closed = false;
    const user = userEvent.setup();
    render(
      <Wrap>
        <EditTopicDialog
          topic={topic}
          open={true}
          onOpenChange={(o) => { if (!o) closed = true; }}
        />
      </Wrap>,
    );
    const desc = screen.getByLabelText(/what to watch for/i);
    await user.clear(desc);
    await user.type(desc, "Changed description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    expect(await screen.findByText(/embedder/i)).toBeInTheDocument();
    expect(closed).toBe(false);
  });

  it("starts from the saved marks and saves the changed ones", async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.get("/api/radar/topics/7/feedback", () =>
        HttpResponse.json({
          items: [{
            verdict: "irrelevant", created_at: "2026-05-18T10:00:00Z",
            finding: rawFinding(3, "Steam Frame starts at $1059"),
          }],
        })),
      http.post("/api/radar/topics/preview", () =>
        HttpResponse.json({
          items: [{ similarity: 0.35, finding: rawFinding(4, "Steam Deck OLED review") }],
          threshold: 0.3,
        })),
      http.patch("/api/radar/topics/7", async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          id: 7, user_id: 1, name: "Old name", description: "Old description, long enough.",
          match_threshold: 0.18, match_cutoff: 0.3, is_active: true, has_embedding: true,
          created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
        });
      }),
    );

    const user = userEvent.setup();
    render(
      <Wrap>
        <EditTopicDialog topic={topic} open={true} onOpenChange={() => {}} />
      </Wrap>,
    );

    expect(await screen.findByText("Marked 0 ✓ · 1 ✗")).toBeInTheDocument();
    const row = (await screen.findByText("Steam Deck OLED review")).closest("li")!;
    await user.click(within(row).getByRole("button", { name: "About this topic" }));
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({
      feedback: [
        { finding_id: 3, verdict: "irrelevant" },
        { finding_id: 4, verdict: "relevant" },
      ],
    });
  });

  it("leaves the marks out of the patch when they were not touched", async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.get("/api/radar/topics/7/feedback", () =>
        HttpResponse.json({
          items: [{
            verdict: "irrelevant", created_at: "2026-05-18T10:00:00Z",
            finding: rawFinding(3, "Steam Frame starts at $1059"),
          }],
        })),
      http.patch("/api/radar/topics/7", async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          id: 7, user_id: 1, name: "New name", description: "Old description, long enough.",
          match_threshold: 0.18, is_active: true, has_embedding: true,
          created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
        });
      }),
    );

    const user = userEvent.setup();
    render(
      <Wrap>
        <EditTopicDialog topic={topic} open={true} onOpenChange={() => {}} />
      </Wrap>,
    );
    await screen.findByText("Marked 0 ✓ · 1 ✗");

    const nameField = screen.getByLabelText(/^name$/i);
    await user.clear(nameField);
    await user.type(nameField, "New name");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({ name: "New name" });
  });
});
