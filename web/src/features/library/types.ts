export type LibraryState = "unread" | "read" | "archived";

export type LibraryItem = {
  id: number;
  state: LibraryState;
  isFavorite: boolean;
  note: string | null;
  savedAt: Date;
  readAt: Date | null;
  url: string;
  title: string | null;
  excerpt: string | null;
  readingTimeSeconds: number | null;
  image: string | null;
};

export type ArticleContent = {
  id: number;
  url: string;
  canonicalUrl: string | null;
  title: string | null;
  byline: string | null;
  excerpt: string | null;
  text: string | null;
  html: string | null;
  lang: string | null;
  readingTimeSeconds: number | null;
  image: string | null;
  favicon: string | null;
  fetchedAt: Date;
  fetchError: string | null;
};

export type LibraryItemDetail = LibraryItem & {
  content: ArticleContent;
};

export type ListPage = {
  items: LibraryItem[];
  total: number;
};

export type FilterParams = {
  state?: LibraryState;
  favorite?: boolean;
};

export const PAGE_SIZE = 20;
