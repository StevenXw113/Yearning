// 登录态持久化（vuex-persistedstate 的替代实现）。
//
// 等价语义（原 vuex-persistedstate paths: ['user.account', 'menu', 'order'],
// sessionStorage）：user/menu/order 三个 store 的状态写回 sessionStorage，
// 刷新后恢复。pinia 的持久化按 store 整体隔离——user 模块 state 仅 account
// 一项，整存整取与原「仅 account」等价，也不会像 vuex 多模块那样互相覆盖。
//
// 为什么不用 pinia-plugin-persistedstate：实测 4.7.1 与 pinia 4.1 组合下插件
// 对 user store 未生效；为什么不用 store.$subscribe：pinia 4.1 的 $subscribe
// 对 action 内整段属性赋值（this.account = vl）不触发回调。改用 Vue 的 watch
// 深度监听 store.$state——响应式基础设施，行为与 vuex-persistedstate 一致。
import { watch } from 'vue';
import { useUserStore, useMenuStore, useOrderStore, pinia } from '@/store';

function wire(
  store: { $patch: (s: unknown) => void; $state: unknown },
  key: string,
) {
  const raw = sessionStorage.getItem(key);
  if (raw) {
    try {
      store.$patch(JSON.parse(raw));
    } catch {
      // 残留的旧/异常结构直接丢弃，走各模块默认值（与原 vuex 迁移注释语义一致）
      sessionStorage.removeItem(key);
    }
  }
  // 整包 JSON.stringify 每次变更都跑,查询历史等高频变更下做节流:
  // 首沿立即写(行为与原来一致),300ms 内的后续变更合并到尾沿补一次,
  // 保证最后状态不丢
  let persistTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingDirty = false;
  const writeNow = () => sessionStorage.setItem(key, JSON.stringify(store.$state));
  watch(
    () => store.$state,
    () => {
      if (persistTimer !== null) {
        pendingDirty = true;
        return;
      }
      writeNow();
      persistTimer = setTimeout(() => {
        persistTimer = null;
        if (pendingDirty) {
          pendingDirty = false;
          writeNow();
        }
      }, 300);
    },
    { deep: true },
  );
}

export function setupSessionPersistence() {
  wire(useUserStore(pinia), 'user');
  wire(useMenuStore(pinia), 'menu');
  wire(useOrderStore(pinia), 'order');
}
