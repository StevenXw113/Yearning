// pinia 入口（vuex → pinia 迁移）。
// 各模块在 store/module/*.ts 用 defineStore 定义。
//
// 组件内直接调用 useXxxStore()；axios 拦截器/路由守卫等非 setup 上下文
// 必须传全局实例：useUserStore(pinia)。
import { createPinia } from 'pinia';

export const pinia = createPinia();

export { useUserStore } from './module/user';
export { useMenuStore } from './module/menu';
export { useOrderStore } from './module/order';
export { useCommonStore } from './module/common';
export { useHighlightStore } from './module/highlight';
