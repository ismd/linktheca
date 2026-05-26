import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { useNewTopicStore } from "../use-new-topic-store";
import { NewTopicDialog } from "./NewTopicDialog";

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

const rawTopic = {
  id: 1, user_id: 1, name: "X", description: "Long enough description.",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
};

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
  useNewTopicStore.getState().open();
});

describe("NewTopicDialog", () => {
  it("submits create and closes on success", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(rawTopic, { status: 201 })),
    );
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.type(screen.getByLabelText(/name/i), "Local-first");
    await user.type(
      screen.getByLabelText(/description/i),
      "CRDTs and offline-first tooling, the user-owned data movement.",
    );
    await user.click(screen.getByRole("button", { name: /save/i }));

    // dialog closes
    await screen.findByText(/saved/i);
    expect(useNewTopicStore.getState().isOpen).toBe(false);
  });

  it("shows specific error on 503 embedder_unavailable and stays open", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(
          { code: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.type(screen.getByLabelText(/name/i), "X");
    await user.type(screen.getByLabelText(/description/i), "Long enough description for radar.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    expect(await screen.findByText(/embedder/i)).toBeInTheDocument();
    expect(useNewTopicStore.getState().isOpen).toBe(true);
  });

  it("validates name and description before submit", async () => {
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument();
  });
});
