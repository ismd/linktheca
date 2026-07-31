import { z } from "zod";
import type {
  LibraryItem,
  LibraryItemDetail,
  ArticleContent,
  ListPage,
} from "./types";

export const RawItemSchema = z.object({
  id: z.number().int(),
  state: z.enum(["unread", "read", "archived"]),
  is_favorite: z.boolean(),
  note: z.string().nullable().optional(),
  saved_at: z.string(),
  read_at: z.string().nullable().optional(),
  url: z.string(),
  title: z.string().nullable().optional(),
  excerpt: z.string().nullable().optional(),
  reading_time_seconds: z.number().int().nullable().optional(),
  image: z.string().nullable().optional(),
});

export const RawContentSchema = z.object({
  id: z.number().int(),
  url: z.string(),
  canonical_url: z.string().nullable().optional(),
  title: z.string().nullable().optional(),
  byline: z.string().nullable().optional(),
  excerpt: z.string().nullable().optional(),
  text: z.string().nullable().optional(),
  html: z.string().nullable().optional(),
  lang: z.string().nullable().optional(),
  reading_time_seconds: z.number().int().nullable().optional(),
  image: z.string().nullable().optional(),
  favicon: z.string().nullable().optional(),
  fetched_at: z.string(),
  fetch_error: z.string().nullable().optional(),
});

export const RawItemDetailSchema = RawItemSchema.extend({
  content: RawContentSchema,
});

export const RawListPageSchema = z.object({
  items: z.array(RawItemSchema),
  total: z.number().int(),
});

export type RawItem = z.infer<typeof RawItemSchema>;
export type RawItemDetail = z.infer<typeof RawItemDetailSchema>;
export type RawListPage = z.infer<typeof RawListPageSchema>;

function nn<T>(v: T | null | undefined): T | null {
  return v ?? null;
}

export function mapItem(raw: RawItem): LibraryItem {
  return {
    id: raw.id,
    state: raw.state,
    isFavorite: raw.is_favorite,
    note: nn(raw.note),
    savedAt: new Date(raw.saved_at),
    readAt: raw.read_at ? new Date(raw.read_at) : null,
    url: raw.url,
    title: nn(raw.title),
    excerpt: nn(raw.excerpt),
    readingTimeSeconds: nn(raw.reading_time_seconds),
    image: nn(raw.image),
  };
}

export function mapContent(raw: z.infer<typeof RawContentSchema>): ArticleContent {
  return {
    id: raw.id,
    url: raw.url,
    canonicalUrl: nn(raw.canonical_url),
    title: nn(raw.title),
    byline: nn(raw.byline),
    excerpt: nn(raw.excerpt),
    text: nn(raw.text),
    html: nn(raw.html),
    lang: nn(raw.lang),
    readingTimeSeconds: nn(raw.reading_time_seconds),
    image: nn(raw.image),
    favicon: nn(raw.favicon),
    fetchedAt: new Date(raw.fetched_at),
    fetchError: nn(raw.fetch_error),
  };
}

export function mapDetail(raw: RawItemDetail): LibraryItemDetail {
  return { ...mapItem(raw), content: mapContent(raw.content) };
}

export function mapListPage(raw: RawListPage): ListPage {
  return { items: raw.items.map(mapItem), total: raw.total };
}
