import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
  matchThreshold: 0.55, isActive: true, hasEmbedding: true,
  createdAt: new Date(), updatedAt: new Date(),
  stats: { newCount: 0, totalCount: 0, sourceCount: 0, lastMatchAt: null },
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

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
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

    const descField = screen.getByLabelText(/description/i) as HTMLTextAreaElement;
    expect(descField.value).toBe("Old description, long enough.");
    await user.clear(descField);
    await user.type(descField, "New description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({ description: "New description, long enough." });
  });

  it("shows specific error on 503 embedder and stays open", async () => {
    server.use(
      http.patch("/api/radar/topics/7", () =>
        HttpResponse.json(
          { code: "embedder_unavailable", message: "embedding service is unavailable" },
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
    const desc = screen.getByLabelText(/description/i);
    await user.clear(desc);
    await user.type(desc, "Changed description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    expect(await screen.findByText(/embedder/i)).toBeInTheDocument();
    expect(closed).toBe(false);
  });
});
