import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { FindingReader } from "./FindingReader";

const rawFinding = {
  id: 100, feed_id: 5, feed_title: "Hacker News",
  url: "https://x.example/a", title: "Tiny Brutalism", summary: "Summary text",
  published_at: "2026-05-17T10:00:00Z",
  discovered_at: "2026-05-18T09:00:00Z",
  content: {
    html: "<p>Full <em>article</em> body</p>", text: "Full article body",
    byline: "Ann Author", site_name: "X", reading_time_seconds: 300,
  },
};

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("FindingReader", () => {
  it("shows the downloaded article of a finding", async () => {
    server.use(
      http.get("/api/radar/findings/100", () => HttpResponse.json(rawFinding)),
    );
    render(<Wrap><FindingReader findingId={100} /></Wrap>);

    expect(await screen.findByRole("heading", { name: "Tiny Brutalism" })).toBeInTheDocument();
    expect(screen.getByText("article").tagName).toBe("EM");
    expect(screen.getByText("Ann Author")).toBeInTheDocument();
    expect(screen.getByText("Hacker News")).toBeInTheDocument();
    expect(screen.queryByText("Summary text")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /back to radar/i })).toHaveAttribute("href", "/radar");
  });

  it("says so when the finding is outside the user's feeds", async () => {
    server.use(
      http.get("/api/radar/findings/100", () =>
        HttpResponse.json({ error: "not_found", message: "not found" }, { status: 404 })),
    );
    render(<Wrap><FindingReader findingId={100} /></Wrap>);

    expect(await screen.findByText("Article not found")).toBeInTheDocument();
  });
});
