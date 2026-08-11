import { useMutation, useQueryClient } from "@tanstack/react-query";
import { saveLink, updateItem, deleteItem, type UpdateInput } from "./api";
import { libraryKeys } from "./use-library";
import type { LibraryItem, ListPage } from "./types";

type InfiniteListData = {
  pages: ListPage[];
  pageParams: unknown[];
};

export function useSaveLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => saveLink(url),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: libraryKeys.all });
    },
  });
}

type UpdateArgs = { id: number; input: UpdateInput };

type RollbackCtx = {
  previousLists: [readonly unknown[], InfiniteListData | undefined][];
  previousItem: LibraryItem | undefined;
};

export function useUpdateItem() {
  const qc = useQueryClient();
  return useMutation<LibraryItem, Error, UpdateArgs, RollbackCtx>({
    mutationFn: ({ id, input }) => updateItem(id, input),
    onMutate: async ({ id, input }) => {
      await qc.cancelQueries({ queryKey: libraryKeys.all });

      const previousLists = qc.getQueriesData<InfiniteListData>({
        queryKey: libraryKeys.lists,
      });
      const previousItem = qc.getQueryData<LibraryItem>(libraryKeys.item(id));

      // Patch every cached list page that contains this item.
      previousLists.forEach(([key, data]) => {
        if (!data) return;
        qc.setQueryData<InfiniteListData>(key, {
          ...data,
          pages: data.pages.map((page) => ({
            ...page,
            items: page.items.map((it) =>
              it.id === id ? applyPatch(it, input) : it,
            ),
          })),
        });
      });

      // Patch single-item cache if present.
      if (previousItem) {
        qc.setQueryData<LibraryItem>(libraryKeys.item(id), applyPatch(previousItem, input));
      }

      return { previousLists, previousItem };
    },
    onError: (_err, vars, ctx) => {
      ctx?.previousLists.forEach(([key, data]) => {
        qc.setQueryData(key, data);
      });
      if (ctx?.previousItem !== undefined) {
        qc.setQueryData(libraryKeys.item(vars.id), ctx.previousItem);
      }
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: libraryKeys.detail(vars.id) });
      qc.invalidateQueries({ queryKey: libraryKeys.lists });
    },
  });
}

function applyPatch(item: LibraryItem, input: UpdateInput): LibraryItem {
  return {
    ...item,
    state: input.state ?? item.state,
    isFavorite: input.isFavorite ?? item.isFavorite,
    note: input.note === undefined ? item.note : input.note,
  };
}

export function useDeleteItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteItem(id),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: libraryKeys.detail(id) });
      qc.removeQueries({ queryKey: libraryKeys.item(id) });
      qc.invalidateQueries({ queryKey: libraryKeys.lists });
    },
  });
}
