import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createTopic,
  updateTopic,
  deleteTopic,
  updateMatch,
  type CreateTopicInput,
  type UpdateTopicInput,
} from "./api";
import { radarKeys } from "./use-radar";
import type { Topic, TopicWithStats } from "./types";

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
    },
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
