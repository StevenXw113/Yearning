import { defineStore } from 'pinia';
import { OrderTableData } from '@/types';

export interface orderStore {
  order: OrderTableData;
  enabled: boolean;
  history: string[];
}

// pinia 迁移（原 vuex order 模块）：整模块持久化到 sessionStorage
export const useOrderStore = defineStore('order', {
  state: (): orderStore => ({
    order: {} as OrderTableData,
    enabled: true,
    history: [] as string[],
  }),
  actions: {
    ORDER_STORE(vl: OrderTableData) {
      this.order = vl;
    },
    SET_ORDER_STATUS(vl: number) {
      this.order.status = vl;
    },
    IS_ENABLED(vl: boolean) {
      this.enabled = vl;
    },
    SET_QUERY_HISTORY(vl: string) {
      if (vl === '') {
        return;
      }
      if (this.history.length < 100) {
        this.history.unshift(vl);
      } else {
        this.history.pop();
        this.history.unshift(vl);
      }
    },
  },
});
