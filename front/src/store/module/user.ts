import { defineStore } from 'pinia';
import { LoginRespPayload } from '@/types';

export interface userStore {
  account: LoginRespPayload;
}

// pinia 迁移（原 vuex user 模块）：持久化到 sessionStorage。
// 原 vuex 用 paths: ['user.account'] 只持久化 account——pinia 的持久化按
// store 整体隔离（不会像 vuex 多模块那样互相覆盖），本模块 state 仅 account
// 一项，整存整取语义等价。实测 4.7.1 的 pick 在整段 state 替换时漏写，故不使用。
export const useUserStore = defineStore('user', {
  state: (): userStore => ({
    account: {
      token: '',
      real_name: '',
      user: '',
      is_record: 2,
    } as LoginRespPayload,
  }),
  actions: {
    USER_STORE(vl: LoginRespPayload) {
      this.account = vl;
    },
  },
});
