import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { AddFeedDialog } from "./AddFeedDialog";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper };
}

beforeEach(() => {
  useAuthStore.getState().setSession("t", {
    id: 1,
    email: "a@example.com",
    displayName: "A",
    isAdmin: true,
  });
});

describe("AddFeedDialog", () => {
  it("shows the duplicate error inline", async () => {
    server.use(
      http.post("/api/radar/feeds", () =>
        HttpResponse.json(
          { error: "duplicate", message: "resource already exists" },
          { status: 409 },
        ),
      ),
    );

    const { wrapper } = makeWrapper();
    render(<AddFeedDialog open onOpenChange={() => {}} />, { wrapper });

    await userEvent.type(
      screen.getByLabelText(/feed url/i),
      "https://theverge.com/rss",
    );
    await userEvent.click(screen.getByRole("button", { name: /add feed/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /already in the catalog/i,
    );
  });

  it("sends the url and the chosen interval", async () => {
    let captured: unknown = null;
    server.use(
      http.post("/api/radar/feeds", async ({ request }) => {
        captured = await request.json();
        return new HttpResponse(null, { status: 201 });
      }),
    );

    const { wrapper } = makeWrapper();
    render(<AddFeedDialog open onOpenChange={() => {}} />, { wrapper });

    await userEvent.type(
      screen.getByLabelText(/feed url/i),
      "https://theverge.com/rss",
    );
    await userEvent.selectOptions(screen.getByLabelText(/check/i), "21600");
    await userEvent.click(screen.getByRole("button", { name: /add feed/i }));

    await screen.findByRole("button", { name: /add feed/i });
    expect(captured).toEqual({
      url: "https://theverge.com/rss",
      fetch_interval_seconds: 21600,
    });
  });

  it("rejects a non-url before hitting the network", async () => {
    const { wrapper } = makeWrapper();
    render(<AddFeedDialog open onOpenChange={() => {}} />, { wrapper });

    await userEvent.type(screen.getByLabelText(/feed url/i), "not-a-url");
    await userEvent.click(screen.getByRole("button", { name: /add feed/i }));

    expect(await screen.findByText(/valid http\(s\) url/i)).toBeInTheDocument();
  });
});
