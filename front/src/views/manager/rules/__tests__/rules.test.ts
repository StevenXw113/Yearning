import { describe, it, expect } from 'vitest';
import {
  rule as rules,
  RULE_CATEGORIES,
  filterRules,
  categoryCounts,
  type RuleCategory,
} from '../rules';

// 规则分类契约:七类页签的数据面。分类是纯前端展示维度(不进存储、不影响引擎),
// 这里锁死:每条规则都归了类、没有空类、计数自洽、过滤正确。
describe('规则分类契约', () => {
  it('七类页签顺序与名单稳定', () => {
    expect(RULE_CATEGORIES.map((c) => c.key)).toEqual([
      'statement',
      'table',
      'column',
      'index',
      'naming',
      'system',
      'engine',
    ]);
  });

  it('每条规则都归类,且没有空类(空类不该出现在页签上)', () => {
    const counts = categoryCounts(rules);
    let sum = 0;
    for (const c of RULE_CATEGORIES) {
      expect(counts[c.key], `分类 ${c.key} 不应为空`).toBeGreaterThan(0);
      sum += counts[c.key];
    }
    expect(sum).toBe(rules.length);
    expect(counts.all).toBe(rules.length);
  });

  it('filterRules:all 返回全量,分类只含该类', () => {
    expect(filterRules(rules, 'all')).toEqual(rules);
    for (const c of RULE_CATEGORIES) {
      const got = filterRules(rules, c.key);
      expect(got.every((r) => r.category === c.key)).toBe(true);
      expect(got.length).toBe(categoryCounts(rules)[c.key]);
    }
  });

  it('代表性规则的归类抽查(SQL/Mongo/Redis/Doris 四个来源各抽)', () => {
    const want: Record<string, RuleCategory> = {
      DMLWhere: 'statement',
      DDLCheckTableComment: 'table',
      DDLMaxCharLength: 'column',
      DDLMaxKeyParts: 'index',
      MaxTableNameLen: 'naming',
      SupportCharset: 'system',
      MaxAffectRows: 'engine',
      MongoForbidDangerous: 'statement',
      MongoCollectionPrefix: 'naming',
      DorisKeyModel: 'table',
      RedisRequireTTL: 'system',
      OscSize: 'engine',
    };
    for (const [name, cat] of Object.entries(want)) {
      const r = rules.find((x) => x.name === name);
      expect(r, `规则 ${name} 应存在于名单`).toBeTruthy();
      expect(r!.category, `规则 ${name} 归类`).toBe(cat);
    }
  });
});
