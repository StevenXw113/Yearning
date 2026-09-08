import { InjectionKey } from 'vue';
import { createStore, useStore as baseUseStore, Store } from 'vuex';
import { menu } from '@/store/module/menu';
import { user } from '@/store/module/user';
import { order } from './module/order';
import { common } from './module/common';
import { highlight } from './module/highlight';
import { RootStore, AllStoreTypes } from './types';
import createPersistedState from 'vuex-persistedstate';

export const store = createStore<AllStoreTypes>({
  modules: {
    menu,
    user,
    order,
    common,
    highlight,
  },
  plugins: [
    // 仅持久化 user.account，避免 storage 中残留的旧/异常结构被整体回写覆盖 user 模块，
    // 导致 store.state.user.account 变成 null/undefined 引发登录页路由/拦截器崩溃
    createPersistedState({
      paths: ['user.account', 'menu', 'order'],
      storage: window.sessionStorage,
    }),
  ],
});

export const key: InjectionKey<Store<RootStore>> = Symbol('vue-store');

// 定义自己的 `useStore` 组合式函数
export function useStore<T = AllStoreTypes>() {
  return baseUseStore<T>(key);
}
