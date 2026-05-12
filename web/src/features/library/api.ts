import { apiFetch } from "@/shared/api/client";
import {
  RawItemSchema,
  RawItemDetailSchema,
  RawListPageSchema,
  mapItem,
  mapDetail,
  mapListPage,
} from "./schemas";
import type {
  LibraryItem,
  LibraryItemDetail,
  ListPage,
  LibraryState,
} from "./types";

export type ListArgs = {
  limit: number;
  offset: number;
  state?: LibraryState;
  favorite?: boolean;
};

function buildQuery(args: ListArgs): string {
  const p = new URLSearchParams();
  p.set("limit", String(args.limit));
  p.set("offset", String(args.offset));
  if (args.state) p.set("state", args.state);
  if (args.favorite !== undefined) p.set("favorite", String(args.favorite));
  return p.toString();
}

function parseInDev<T>(schema: { parse: (x: unknown) => T }, data: unknown): T {
  if (import.meta.env.DEV || import.meta.env.MODE === "test") {
    return schema.parse(data);
  }
  return data as T;
}

export async function listLibrary(args: ListArgs): Promise<ListPage> {
  const raw = await apiFetch<unknown>(`/library?${buildQuery(args)}`);
  return mapListPage(parseInDev(RawListPageSchema, raw));
}

export async function getLibraryItem(id: number): Promise<LibraryItem> {
  const raw = await apiFetch<unknown>(`/library/${id}`);
  return mapItem(parseInDev(RawItemSchema, raw));
}

export async function getLibraryDetail(id: number): Promise<LibraryItemDetail> {
  const raw = await apiFetch<unknown>(`/library/${id}/content`);
  return mapDetail(parseInDev(RawItemDetailSchema, raw));
}

export async function saveLink(url: string): Promise<LibraryItem> {
  const raw = await apiFetch<unknown>(`/library`, {
    method: "POST",
    body: JSON.stringify({ url }),
  });
  return mapItem(parseInDev(RawItemSchema, raw));
}

export type UpdateInput = {
  state?: LibraryState;
  isFavorite?: boolean;
  note?: string | null;
};

export async function updateItem(
  id: number,
  input: UpdateInput,
): Promise<LibraryItem> {
  const body: Record<string, unknown> = {};
  if (input.state !== undefined) body.state = input.state;
  if (input.isFavorite !== undefined) body.is_favorite = input.isFavorite;
  if (input.note !== undefined) body.note = input.note;
  const raw = await apiFetch<unknown>(`/library/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return mapItem(parseInDev(RawItemSchema, raw));
}

export async function deleteItem(id: number): Promise<void> {
  await apiFetch<void>(`/library/${id}`, { method: "DELETE" });
}
