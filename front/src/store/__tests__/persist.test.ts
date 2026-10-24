import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia } from 'pinia';
import { nextTick } from 'vue';
import { setupSessionPersistence } from '@/store/persist';
import { pinia, useUserStore, useMenuStore, useOrderStore } from '@/store';

// 会话持久化是登录态的核心：pinia 手写持久化（watch $state ↔ sessionStorage）。
// persist 挂在应用的全局 pinia 上，测试直接复用该实例（setActivePinia(app)），
// 每例前 $reset + 清 storage 保证隔离。写回断言需 await nextTick()（watch 默认 pre-flush）。
describe('session persistence', () => {
  beforeEach(() => {
    sessionStorage.clear();
    setActivePinia(pinia);
    useUserStore().$reset();
    useMenuStore().$reset();
    useOrderStore().$reset();
  });

  it('action 变更写回 sessionStorage', async () => {
    setupSessionPersistence();
    useUserStore().USER_STORE({
      token: 'tk-1',
      user: 'alice',
      real_name: 'A',
      is_record: 1,
      rule: '',
    });
    await nextTick();
    const parsed = JSON.parse(sessionStorage.getItem('user')!);
    expect(parsed.account.token).toBe('tk-1');
    expect(parsed.account.user).toBe('alice');
  });

  it('启动时从 sessionStorage 恢复（重挂持久化后读到旧会话）', () => {
    sessionStorage.setItem(
      'user',
      JSON.stringify({ account: { token: 'tk-2', user: 'bob', real_name: 'B', is_record: 2, rule: '' } })
    );
    setupSessionPersistence();
    const user = useUserStore();
    expect(user.account.token).toBe('tk-2');
    expect(user.account.user).toBe('bob');
  });

  it('损坏的存储数据被丢弃，不抛异常且清除残留', () => {
    sessionStorage.setItem('user', '{not-json');
    expect(() => setupSessionPersistence()).not.toThrow();
    expect(useUserStore().account.token).toBe('');
    expect(sessionStorage.getItem('user')).toBeNull();
  });

  it('menu/order 同样持久化（整 store 写回）', async () => {
    setupSessionPersistence();
    useMenuStore().CHANGE_SELECTED(['/manager/db']);
    useOrderStore().SET_ORDER_STATUS(4);
    await nextTick();
    expect(sessionStorage.getItem('menu')).toContain('/manager/db');
    expect(sessionStorage.getItem('order')).toContain('"enabled"');
  });
});
