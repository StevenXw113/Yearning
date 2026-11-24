import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Antd from 'ant-design-vue';
import i18n from '@/lang';
import Rules from '../rules.vue';

describe('MongoDB 规则 UI', () => {
  it('显示稳定 ID，并强制 DBA 特批', async () => {
    const wrapper = mount(Rules, {
      props: { desc: '测试', engine: 'mongo' },
      global: { plugins: [Antd, i18n] },
    });
    await wrapper.vm.$nextTick();
    expect(wrapper.text()).toContain('mongo.multi_write');
    expect(wrapper.text()).toContain('mongo.collection.drop');
    expect(wrapper.text()).toContain('mongo.account.privilege');
    expect(wrapper.text()).toContain('DBA');
  });
});
