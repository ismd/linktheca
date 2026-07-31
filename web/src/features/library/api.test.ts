import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  listLibrary,
  getLibraryItem,
  getLibraryDetail,
  saveLink,
  updateItem,
  deleteItem,
} from "./api";

const rawItem = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  state: "unread",
  is_favorite: false,
  note: null,
  saved_at: "2026-05-10T12:00:00Z",
  read_at: null,
  url: "https://example.com/a",
  title: "Example",
  excerpt: "Some excerpt",
  reading_time_seconds: 480,
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1,
    email: "u@x.co",
    displayName: "U",
    isAdmin: false,
  });
});

describe("library api", () => {
  it("listLibrary sends limit/offset/state/favorite as query params", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [rawItem()], total: 1 });
      }),
    );

    const page = await listLibrary({
      limit: 20,
      offset: 40,
      state: "unread",
      favorite: true,
    });

    expect(capturedUrl).toContain("limit=20");
    expect(capturedUrl).toContain("offset=40");
    expect(capturedUrl).toContain("state=unread");
    expect(capturedUrl).toContain("favorite=true");
    expect(page.total).toBe(1);
    const first = page.items[0]!;
    expect(first.savedAt).toBeInstanceOf(Date);
    expect(first.isFavorite).toBe(false);
  });

  it("listLibrary omits filter params when not set", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/library", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );

    await listLibrary({ limit: 20, offset: 0 });
    expect(capturedUrl).not.toContain("state=");
    expect(capturedUrl).not.toContain("favorite=");
  });

  it("getLibraryItem maps the response", async () => {
    server.use(
      http.get("/api/library/42", () => HttpResponse.json(rawItem({ id: 42 }))),
    );
    const item = await getLibraryItem(42);
    expect(item.id).toBe(42);
    expect(item.savedAt).toBeInstanceOf(Date);
  });

  it("getLibraryDetail maps item+content", async () => {
    server.use(
      http.get("/api/library/7/content", () =>
        HttpResponse.json({
          ...rawItem({ id: 7 }),
          content: {
            id: 99,
            url: "https://example.com/a",
            canonical_url: null,
            title: "Example",
            byline: "By Someone",
            excerpt: "Some excerpt",
            text: "Full text",
            html: "<p>Full text</p>",
            lang: "en",
            reading_time_seconds: 480,
            fetched_at: "2026-05-10T12:00:00Z",
            fetch_error: null,
          },
        }),
      ),
    );
    const detail = await getLibraryDetail(7);
    expect(detail.content.html).toBe("<p>Full text</p>");
    expect(detail.content.fetchedAt).toBeInstanceOf(Date);
  });

  it("maps the downloaded image and favicon file names", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({
          items: [rawItem({ image: "a1b2c3.png" })],
          total: 1,
        }),
      ),
      http.get("/api/library/7/content", () =>
        HttpResponse.json({
          ...rawItem({ id: 7, image: "a1b2c3.png" }),
          content: {
            id: 99,
            url: "https://example.com/a",
            fetched_at: "2026-05-10T12:00:00Z",
            image: "a1b2c3.png",
            favicon: "example.com.png",
          },
        }),
      ),
    );

    const page = await listLibrary({ limit: 20, offset: 0 });
    expect(page.items[0]!.image).toBe("a1b2c3.png");

    const detail = await getLibraryDetail(7);
    expect(detail.content.image).toBe("a1b2c3.png");
    expect(detail.content.favicon).toBe("example.com.png");
  });

  it("leaves image and favicon null when the backend omits them", async () => {
    server.use(
      http.get("/api/library", () =>
        HttpResponse.json({ items: [rawItem()], total: 1 }),
      ),
    );

    const page = await listLibrary({ limit: 20, offset: 0 });
    expect(page.items[0]!.image).toBeNull();
  });

  it("saveLink POSTs { url } and returns mapped item", async () => {
    let captured: { url: string } | null = null;
    server.use(
      http.post("/api/library", async ({ request }) => {
        captured = (await request.json()) as { url: string };
        return HttpResponse.json(rawItem({ id: 5 }), { status: 201 });
      }),
    );

    const item = await saveLink("https://example.com/a");
    expect(captured).toEqual({ url: "https://example.com/a" });
    expect(item.id).toBe(5);
  });

  it("updateItem PATCHes and maps response", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ id: 3, is_favorite: true }));
      }),
    );

    const item = await updateItem(3, { isFavorite: true });
    expect(captured).toEqual({ is_favorite: true });
    expect(item.isFavorite).toBe(true);
  });

  it("updateItem maps state and note correctly", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/library/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawItem({ id: 3, state: "read", note: "hi" }));
      }),
    );

    await updateItem(3, { state: "read", note: "hi" });
    expect(captured).toEqual({ state: "read", note: "hi" });
  });

  it("deleteItem DELETEs", async () => {
    let called = false;
    server.use(
      http.delete("/api/library/9", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await deleteItem(9);
    expect(called).toBe(true);
  });
});
