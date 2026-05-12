import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { Toaster } from "@/shared/ui/sonner";
import { useAuthStore } from "@/features/auth/store";
import { useAddLinkStore } from "../use-add-link-store";
import { AddLinkDialog } from "./AddLinkDialog";

function wrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("a", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
  useAddLinkStore.getState().close();
  vi.useRealTimers();
});

const rawItem = {
  id: 1,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/a",
  title: "T",
  excerpt: null,
  reading_time_seconds: null,
};

describe("AddLinkDialog", () => {
  it("does not render content when closed", () => {
    render(<AddLinkDialog />, { wrapper: wrapper() });
    expect(screen.queryByLabelText(/url/i)).not.toBeInTheDocument();
  });

  it("renders form when open", () => {
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    expect(screen.getByLabelText(/url/i)).toBeInTheDocument();
  });

  it("validates URL inline", async () => {
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "not a url");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByText(/valid url/i)).toBeInTheDocument();
  });

  it("on success: closes dialog and shows toast", async () => {
    server.use(
      http.post("/api/library", () => HttpResponse.json(rawItem, { status: 201 })),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() =>
      expect(useAddLinkStore.getState().isOpen).toBe(false),
    );
    expect(await screen.findByText(/saved to library/i)).toBeInTheDocument();
  });

  it("on 409 already_saved: shows specific error and stays open", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json(
          { code: "already_saved", message: "already" },
          { status: 409 },
        ),
      ),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/already in your library/i);
    expect(useAddLinkStore.getState().isOpen).toBe(true);
  });

  it("on 5xx: shows generic error", async () => {
    server.use(
      http.post("/api/library", () =>
        HttpResponse.json({ code: "internal", message: "x" }, { status: 500 }),
      ),
    );
    useAddLinkStore.getState().open();
    render(<AddLinkDialog />, { wrapper: wrapper() });
    const u = userEvent.setup();
    await u.type(screen.getByLabelText(/url/i), "https://example.com/a");
    await u.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/couldn't save/i);
  });
});
