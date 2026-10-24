import { defineStore } from 'pinia';

export interface lightWord {
  meta: string;
  vl: string;
}

export interface highlightStore {
  highlight: { [key: string]: lightWord[] };
}

// pinia 迁移（原 vuex highlight 模块）：不持久化
export const useHighlightStore = defineStore('highlight', {
  state: (): highlightStore => ({
    highlight: {},
  }),
  actions: {
    SAVE_HIGHLIGHT(word: { key: string; highlight: lightWord[] }) {
      this.highlight[word.key] = word.highlight;
    },
  },
});
