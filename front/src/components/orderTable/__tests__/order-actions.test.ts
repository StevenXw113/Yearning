import { describe, it, expect, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import Antd from 'ant-design-vue';
import { nextTick } from 'vue';
import dayjs from 'dayjs';
import i18n from '@/lang';
import Search from '../orderTableSearch.vue';
import OrderTable from '../orderTable.vue';
import CTable from '@/components/table/table.vue';

const socket = vi.hoisted(() => ({ onMessage: undefined as any }));
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => ({ params: { tp: 'record' } }),
  onBeforeRouteUpdate: vi.fn(),
}));
vi.mock('@/store', () => ({
  useUserStore: () => ({ account: { token: 'test' } }),
  useOrderStore: () => ({ ORDER_STORE: vi.fn() }),
}));
vi.mock('@/apis/source', () => ({
  querySourceList: vi.fn().mockResolvedValue({ data: { payload: [] } }),
}));
vi.mock('@vueuse/core', async (original) => ({
  ...(await original<typeof import('@vueuse/core')>()),
  useWebSocket: (_url: string, options: any) => {
    socket.onMessage = options.onMessage;
    return { send: vi.fn() };
  },
}));
const global = {
  plugins: [Antd, i18n],
  components: { CTable },
  stubs: { Profile: true, Delay: true },
};

describe('order action controls', () => {
  it('formats submitted dates without replacing the date-picker values', async () => {
    const wrapper = mount(Search, { global });
    const form = wrapper.findComponent({ name: 'AForm' });
    const model = form.props('model') as any;
    model.picker = [dayjs('2026-11-01 10:20'), dayjs('2026-11-02 11:30')];
    await form.vm.$emit('finish');
    expect(wrapper.emitted('search')?.[0]?.[0]).toMatchObject({
      picker: ['2026-11-01 10:20', '2026-11-02 11:30'],
    });
    expect(dayjs.isDayjs(model.picker[0])).toBe(true);
    wrapper.unmount();
  });

  it('expands accessible filters and resets all query conditions', async () => {
    const wrapper = mount(Search, { global });
    const expand = wrapper.find('button[aria-expanded]');
    await expand.trigger('click');
    expect(expand.attributes('aria-expanded')).toBe('true');
    const model = wrapper
      .findComponent({ name: 'AForm' })
      .props('model') as any;
    model.username = 'someone';
    model.status = 2;
    model.type = 1;
    model.work_id = '12';
    const reset = wrapper
      .findAll('button')
      .find((button) => button.text().replace(/\s/g, '') === '重置')!;
    await reset.trigger('click');
    const searches = wrapper.emitted('search') || [];
    expect(searches[searches.length - 1]?.[0]).toMatchObject({
      username: '',
      status: 8,
      type: 2,
      work_id: '',
    });
    wrapper.unmount();
  });

  it('expands and collapses batch rows through the action button while keeping the action column fixed', async () => {
    const wrapper = mount(OrderTable, { global });
    await flushPromises();
    socket.onMessage(
      {},
      {
        data: JSON.stringify({
          payload: {
            data: [
              {
                work_id: 'a',
                batch_id: 'batch',
                text: 'change',
                type: 1,
                status: 2,
                delay: 'none',
              },
              {
                work_id: 'b',
                batch_id: 'batch',
                text: 'change',
                type: 1,
                status: 2,
                delay: 'none',
              },
            ],
            page: 2,
          },
        }),
      },
    );
    await nextTick();
    await wrapper.find('.order-project-action').trigger('click');
    await nextTick();
    expect(
      wrapper.find('.order-project-action').attributes('aria-expanded'),
    ).toBe('true');
    expect(wrapper.findAll('.order-row-actions')).toHaveLength(2);
    const actionColumn = wrapper
      .findComponent({ name: 'ATable' })
      .props('columns')
      .find((column: any) => column.dataIndex === 'action');
    expect(actionColumn).toMatchObject({ fixed: 'right', resizable: false });
    await wrapper.find('.order-row-actions button').trigger('click');
    expect(
      wrapper.find('.order-project-action').attributes('aria-expanded'),
    ).toBe('true');
    await wrapper.find('.order-project-action').trigger('click');
    await nextTick();
    expect(
      wrapper.find('.order-project-action').attributes('aria-expanded'),
    ).toBe('false');
    expect(
      wrapper.findAll('.order-row-actions').filter((row) => row.isVisible()),
    ).toHaveLength(0);
    wrapper.unmount();
  });
});
