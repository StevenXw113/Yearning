import { describe, it, expect, beforeEach } from 'vitest';
import { OrderState } from '@/types';
import { useOrderStore } from '@/store/module/order';
import { createPinia, setActivePinia } from 'pinia';

// 前端侧的状态契约：OrderState 枚举值与后端 model.OrderStatus* 常量一一对应
// （后端 Go 侧的对照测试在 src/archguard）。此处锁死前端枚举不被无意改动/重排，
// 并覆盖 order store 的查询历史环形缓冲逻辑。
describe('OrderState contract (frontend side)', () => {
  it('枚举值与后端契约一致(0..6)', () => {
    expect(OrderState.REJECT).toBe(0);
    expect(OrderState.SUCCESS).toBe(1);
    expect(OrderState.AUDIT).toBe(2);
    expect(OrderState.PROCESS).toBe(3);
    expect(OrderState.ERROR).toBe(4);
    expect(OrderState.WAIT).toBe(5);
    expect(OrderState.Undo).toBe(6);
  });
});

describe('order store: SET_QUERY_HISTORY 环形缓冲', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('空串不入历史', () => {
    const s = useOrderStore();
    s.SET_QUERY_HISTORY('');
    expect(s.history).toHaveLength(0);
  });

  it('新查询插入队头', () => {
    const s = useOrderStore();
    s.SET_QUERY_HISTORY('SELECT 1');
    s.SET_QUERY_HISTORY('SELECT 2');
    expect(s.history).toEqual(['SELECT 2', 'SELECT 1']);
  });

  it('超过 100 条时淘汰最旧的（环容量恒为 100）', () => {
    const s = useOrderStore();
    for (let i = 0; i < 105; i++) s.SET_QUERY_HISTORY(`q${i}`);
    expect(s.history).toHaveLength(100);
    expect(s.history[0]).toBe('q104'); // 最新在队头
    expect(s.history[99]).toBe('q5'); // q0..q4 被淘汰
  });
});
