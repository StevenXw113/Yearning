import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import Antd from 'ant-design-vue';
import i18n from '@/lang';

vi.mock('@/apis/policy', () => ({
  getPolicySources: vi.fn().mockResolvedValue({
    data: {
      code: 1200,
      payload: {
        source: [
          { source: 'dev-db-1', source_id: 'a1' },
          { source: 'dev-db-2', source_id: 'a2' },
        ],
        query: [{ source: 'dev-db-1', source_id: 'a1' }],
      },
    },
  }),
  updatePolicy: vi.fn(),
}));

import PolicyModal from '../policyModal.vue';

// 回归点（用户反馈的三个现象）：
//   1) 界面重复显示 → 三个维度各一个穿梭框，不能多渲染
//   2) 具体权限无选项 → 列表项要显示数据源名称
//   3) 选择不生效 → target-keys 双向绑定要能写回
async function openModal() {
  const w = mount(PolicyModal, {
    props: { title: '新建权限组' },
    global: { plugins: [Antd, i18n] },
    attachTo: document.body,
  });
  await Promise.resolve();
  (w.vm as unknown as { newPolicy: () => void }).newPolicy();
  await w.vm.$nextTick();
  await w.vm.$nextTick();
  return w;
}

describe('policyModal 权限组弹窗', () => {
  it('三个维度各渲染一个穿梭框', async () => {
    const w = await openModal();
    expect(w.findAllComponents({ name: 'ATransfer' })).toHaveLength(3);
    w.unmount();
  });

  it('列表项显示数据源名称，且数量正确', async () => {
    const w = await openModal();
    const items = document.querySelectorAll('.ant-transfer-list-content-item');
    // ddl(2) + dml(2) + query(1)
    expect(items).toHaveLength(5);
    expect(document.body.textContent).toContain('dev-db-2');
    w.unmount();
    document.body.innerHTML = '';
  });

  it('移动到右侧后 target-keys 写回权限字段', async () => {
    const w = await openModal();
    const transfers = w.findAllComponents({ name: 'ATransfer' });
    await transfers[0].vm.$emit('update:target-keys', ['a1']);
    await w.vm.$nextTick();
    expect(transfers[0].props('targetKeys')).toEqual(['a1']);
    w.unmount();
    document.body.innerHTML = '';
  });
});
