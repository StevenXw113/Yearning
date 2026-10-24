import { defineStore } from 'pinia';

export interface menuStore {
  selectedKey: string[];
  activeKey: string;
}

// pinia 迁移（原 vuex menu 模块）：整模块持久化到 sessionStorage
export const useMenuStore = defineStore('menu', {
  state: (): menuStore => ({
    selectedKey: ['/home'],
    activeKey: 'dml',
  }),
  actions: {
    CHANGE_SELECTED(vl: string[]) {
      this.selectedKey = vl;
    },
    CHANGE_ACTIVE_TABS(vl: string) {
      this.activeKey = vl;
    },
  },
});
