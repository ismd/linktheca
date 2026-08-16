export type MatchState = "new" | "seen";

export type TopicStats = {
  newCount: number;
  totalCount: number;
  sourceCount: number;
  lastMatchAt: Date | null;
};

export type Topic = {
  id: number;
  userId: number;
  name: string;
  description: string;
  matchThreshold: number;
  isActive: boolean;
  hasEmbedding: boolean;
  createdAt: Date;
  updatedAt: Date;
};

export type TopicWithStats = Topic & {
  stats: TopicStats;
};

export type MatchFinding = {
  id: number;
  feedId: number;
  feedTitle: string | null;
  url: string;
  title: string | null;
  summary: string | null;
  publishedAt: Date | null;
  discoveredAt: Date;
};

export type MatchView = {
  id: number;
  topicId: number;
  topicName: string;
  similarity: number;
  state: MatchState;
  matchedAt: Date;
  finding: MatchFinding;
};

export type PreviewMatch = {
  similarity: number;
  finding: MatchFinding;
};

export type TopicPreview = {
  items: PreviewMatch[];
  threshold: number;
};

export type MatchList = {
  items: MatchView[];
  total: number;
};

export type RadarStatus = {
  lastSweepAt: Date | null;
};

export type MatchFilters = {
  topicId?: number;
  state?: MatchState;
};

export type InboxState = "new" | "all";

export type InboxFilters = {
  state: InboxState;
  topicId?: number;
};

export const PAGE_SIZE = 20;

export type FeedListItem = {
  id: number;
  url: string;
  kind: string;
  title: string | null;
  fetchIntervalSeconds: number;
  isActive: boolean;
  lastFetchedAt: Date | null;
  lastError: string | null;
  createdAt: Date;
  subscribed: boolean;
  findingCount: number;
};
