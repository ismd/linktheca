export type MatchState = "new" | "seen";

/** The user's mark on a finding for one topic. */
export type Verdict = "relevant" | "irrelevant";

export type TopicStats = {
  newCount: number;
  totalCount: number;
  sourceCount: number;
  lastMatchAt: Date | null;
  /** How many findings the user marked as about the topic, and as not. */
  markedRelevant: number;
  markedIrrelevant: number;
};

export type Topic = {
  id: number;
  userId: number;
  name: string;
  description: string;
  /** The base the user sets. */
  matchThreshold: number;
  /** What findings are held to after the topic learned from the marks. */
  matchCutoff: number;
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
  /** The feed's description, or the page excerpt when the feed has none. */
  summary: string | null;
  publishedAt: Date | null;
  discoveredAt: Date;
  /** The downloaded article. Only a single match carries it, and only while
   * the page is stored. */
  content: FindingContent | null;
};

export type FindingContent = {
  /** Sanitized by the backend; safe to render as HTML. */
  html: string | null;
  text: string | null;
  byline: string | null;
  siteName: string | null;
  readingTimeSeconds: number | null;
};

export type MatchView = {
  id: number;
  topicId: number;
  topicName: string;
  similarity: number;
  state: MatchState;
  matchedAt: Date;
  verdict: Verdict | null;
  finding: MatchFinding;
};

/** A mark together with the finding it is on, so a dialog can list it. */
export type TopicMark = {
  verdict: Verdict;
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
  maxUserFeeds: number | null;
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
  isOwn: boolean;
};
