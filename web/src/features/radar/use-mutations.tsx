import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createTopic,
  updateTopic,
  deleteTopic,
  updateMatch,
  subscribeFeed,
  unsubscribeFeed,
  addFeed,
  updateFeed,
  deleteFeed,
  setTopicMark,
  deleteTopicMark,
  type CreateTopicInput,
  type UpdateTopicInput,
  type AddFeedInput,
  type UpdateFeedInput,
} from "./api";
import { radarKeys } from "./use-radar";
import type { Topic, TopicWithStats, FeedListItem, Verdict } from "./types";

type UpdateArgs = { id: number; input: UpdateTopicInput };

type RollbackCtx = {
  previousTopics: TopicWithStats[] | undefined;
  previousTopic: TopicWithStats | undefined;
};

export function useCreateTopic() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTopicInput) => createTopic(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

export function useUpdateTopic() {
  const qc = useQueryClient();
  return useMutation<Topic, Error, UpdateArgs, RollbackCtx>({
    mutationFn: ({ id, input }) => updateTopic(id, input),
    onMutate: async ({ id, input }) => {
      await qc.cancelQueries({ queryKey: radarKeys.topics });
      const previousTopics = qc.getQueryData<TopicWithStats[]>(radarKeys.topics);
      const previousTopic = qc.getQueryData<TopicWithStats>(radarKeys.topic(id));

      if (previousTopics) {
        qc.setQueryData<TopicWithStats[]>(
          radarKeys.topics,
          previousTopics.map((t) => (t.id === id ? patchTopic(t, input) : t)),
        );
      }
      if (previousTopic) {
        qc.setQueryData<TopicWithStats>(
          radarKeys.topic(id),
          patchTopic(previousTopic, input),
        );
      }
      return { previousTopics, previousTopic };
    },
    onError: (_err, vars, ctx) => {
      if (ctx?.previousTopics !== undefined) {
        qc.setQueryData(radarKeys.topics, ctx.previousTopics);
      }
      if (ctx?.previousTopic !== undefined) {
        qc.setQueryData(radarKeys.topic(vars.id), ctx.previousTopic);
      }
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: radarKeys.topic(vars.id) });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
      if (vars.input.marks !== undefined) invalidateMarks(qc, vars.id);
    },
  });
}

/** What a changed mark touches: the topic's counts and cutoff, its marks, and
 * the match lists a ✗ hides a match from. */
function invalidateMarks(qc: ReturnType<typeof useQueryClient>, topicId: number) {
  qc.invalidateQueries({ queryKey: radarKeys.topicMarks(topicId) });
  qc.invalidateQueries({ queryKey: radarKeys.topic(topicId) });
  qc.invalidateQueries({ queryKey: radarKeys.topics });
  qc.invalidateQueries({ queryKey: ["radar", "matches"] });
  qc.invalidateQueries({ queryKey: ["radar", "match"] });
}

type MarkArgs = { topicId: number; findingId: number; verdict: Verdict | null };

/** Sets the user's verdict on a finding for a topic; null takes it back. */
export function useSetTopicMark() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ topicId, findingId, verdict }: MarkArgs) =>
      verdict === null
        ? deleteTopicMark(topicId, findingId)
        : setTopicMark(topicId, findingId, verdict),
    onSettled: (_data, _err, vars) => invalidateMarks(qc, vars.topicId),
  });
}

function patchTopic(t: TopicWithStats, input: UpdateTopicInput): TopicWithStats {
  return {
    ...t,
    name: input.name ?? t.name,
    description: input.description ?? t.description,
    matchThreshold: input.matchThreshold ?? t.matchThreshold,
    isActive: input.isActive ?? t.isActive,
  };
}

export function useDeleteTopic() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteTopic(id),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: radarKeys.topic(id) });
      qc.removeQueries({ queryKey: ["radar", "matches"] });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

export function useMarkMatchSeen() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => updateMatch(id, { state: "seen" }),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: radarKeys.match(id) });
      qc.invalidateQueries({ queryKey: ["radar", "matches"] });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

type ToggleArgs = { feedId: number; subscribed: boolean };

export function useToggleSubscription() {
  const qc = useQueryClient();
  return useMutation<void, Error, ToggleArgs, { previous: FeedListItem[] | undefined }>({
    mutationFn: ({ feedId, subscribed }) =>
      subscribed ? subscribeFeed(feedId) : unsubscribeFeed(feedId),
    onMutate: async ({ feedId, subscribed }) => {
      await qc.cancelQueries({ queryKey: radarKeys.feeds });
      const previous = qc.getQueryData<FeedListItem[]>(radarKeys.feeds);
      if (previous) {
        qc.setQueryData<FeedListItem[]>(
          radarKeys.feeds,
          previous.map((f) => (f.id === feedId ? { ...f, subscribed } : f)),
        );
      }
      return { previous };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous !== undefined) {
        qc.setQueryData(radarKeys.feeds, ctx.previous);
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useAddFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddFeedInput) => addFeed(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useUpdateFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateFeedInput }) =>
      updateFeed(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useDeleteFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteFeed(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}
