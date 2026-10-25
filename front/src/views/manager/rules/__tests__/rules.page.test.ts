import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import Antd from 'ant-design-vue';
import i18n from '@/lang';
import rules from '../rules.vue';

// 规则配置页行为检查:分类页签真渲染、点页签真过滤、搜索与页签叠加。
// 纯函数契约在 rules.test.ts 锁,这里锁组件装配(antd Segmented/i18n/表格联动)。
describe('rules.vue 分类页签', () => {
  const wrapper = mount(rules, {
    props: { desc: '测试' },
    global: { plugins: [Antd, i18n] },
  });

  it('页签渲染八个选项,带 i18n 文案与计数(而非原始 key)', () => {
    const seg = wrapper.findComponent({ name: 'ASegmented' });
    expect(seg.exists()).toBe(true);
    const opts = seg.props('options') as { label: string; value: string }[];
    expect(opts).toHaveLength(8);
    expect(opts[0].label).toContain('全部');
    expect(opts[0].label).toContain('90');
    // i18n 注册失败时 t() 会原样返回 key,这里确保拿到的是翻译文案
    expect(opts[1].label).not.toContain('ruleCat');
    expect(opts[1].label).toContain('语句');
  });

  it('默认全部:表格渲染第一页且分页总数正确(90 条/页大小 20,Mongo 规则不在首页)', () => {
    const text = wrapper.text();
    expect(text).toContain('DDLCheckTableComment');
    expect(text).toContain('90');
    expect(text).not.toContain('MongoForbidDangerous');
  });

  it('切到「表」分类:只剩表类规则', async () => {
    const seg = wrapper.findComponent({ name: 'ASegmented' });
    await seg.vm.$emit('update:value', 'table');
    const text = wrapper.text();
    expect(text).toContain('DDLCheckTableComment');
    expect(text).not.toContain('DMLWhere');
  });

  it('搜索与分类叠加:表分类下搜「注释」只剩表注释规则', async () => {
    const search = wrapper.findComponent({ name: 'AInputSearch' });
    await search.vm.$emit('search', '表注释');
    const text = wrapper.text();
    expect(text).toContain('DDLCheckTableComment');
    expect(text).not.toContain('DDLEnableForeignKey');
  });
});
