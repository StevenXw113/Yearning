import { describe, expect, it } from 'vitest';
import {
  buildMongoRuleSetDefinition,
  isRuleSetCompatibleWithDBType,
  updateMongoRuleSetDefinition,
  mongoRuleIdsFromAuditRole,
  type RuleSetSettings,
} from '@/apis/rules';
import { DBType } from '@/apis/db';

describe('MongoDB 规则集 v2', () => {
  it('稳定 ID 使用 mongo.* 命名空间', () => {
    const ids = mongoRuleIdsFromAuditRole({
      MongoForbidMultiWrite: true,
      MongoForbidDropCollection: true,
      MongoForbidAdminCommand: true,
    });
    expect(ids).toContain('mongo.multi_write');
    expect(ids).toContain('mongo.collection.drop');
    expect(ids).toContain('mongo.collection.rename');
    expect(ids).toContain('mongo.account.privilege');
  });

  it('DBA 硬底线不受普通开关影响', () => {
    const def = buildMongoRuleSetDefinition({
      MongoForbidDropCollection: false,
      MongoForbidAdminCommand: false,
    });
    const ids = def.rules.filter((r) => r.enabled).map((r) => r.id);
    expect(ids).toContain('mongo.collection.drop');
    expect(ids).toContain('mongo.collection.rename');
    expect(ids).toContain('mongo.account.privilege');
    expect(def.dialect).toBe('mongodb');
    expect(def.scope).toBe('data_source');
  });

  it('规则集按数据源类型预过滤', () => {
    expect(
      isRuleSetCompatibleWithDBType(
        { dialect: 'mongodb', audit_role: { MongoForbidMultiWrite: true } } as any,
        DBType.MongoDB,
      ),
    ).toBe(true);
    expect(
      isRuleSetCompatibleWithDBType(
        { dialect: 'mysql', audit_role: {} } as any,
        DBType.MongoDB,
      ),
    ).toBe(false);
    expect(
      isRuleSetCompatibleWithDBType(
        { dialect: 'postgres', audit_role: {} } as any,
        DBType.PostgreSQL,
      ),
    ).toBe(true);
  });

  it('updates an existing MongoDB definition without losing unknown or disabled state', () => {
    const updated = updateMongoRuleSetDefinition(
      {
        schema_version: 2,
        dialect: 'mongodb',
        dialects: ['mongodb'],
        scope: 'data_source',
        settings: {
          max_affect_rows: 1,
          future_setting: 'keep',
        } as RuleSetSettings & { future_setting: string },
        rules: [
          {
            id: 'mongo.future_rule',
            enabled: true,
            level: 'warn',
            reason: 'future',
          },
          {
            id: 'mongo.multi_write',
            enabled: false,
            level: 'warn',
            reason: 'keep disabled',
          },
          {
            id: 'mongo.collection.drop',
            enabled: false,
            level: 'error',
          },
          {
            id: 'mongo.large_in_list',
            enabled: true,
            level: 'warn',
            params: { max_items: 50, future_param: 'keep' },
          },
        ],
      },
      {
        MongoMaxAffectRows: 100,
        MongoLargeInList: 0,
        MongoForbidMultiWrite: false,
        MongoForbidDropCollection: false,
      },
    );

    expect((updated.settings as RuleSetSettings & { future_setting?: string }).future_setting).toBe('keep');
    expect(updated.settings.max_affect_rows).toBe(100);
    expect(updated.rules.some((rule) => rule.id === 'mongo.future_rule')).toBe(true);
    expect(updated.rules.find((rule) => rule.id === 'mongo.multi_write')?.enabled).toBe(false);
    expect(updated.rules.find((rule) => rule.id === 'mongo.collection.drop')?.enabled).toBe(true);
    expect(updated.rules.find((rule) => rule.id === 'mongo.collection.rename')?.enabled).toBe(true);
    expect(updated.rules.find((rule) => rule.id === 'mongo.account.privilege')?.enabled).toBe(true);
    expect(updated.rules.find((rule) => rule.id === 'mongo.large_in_list')?.params).toEqual({
      future_param: 'keep',
    });
  });

  it('keeps legacy SQL compatible with MySQL and PostgreSQL only', () => {
    const legacySQL = { dialect: '', audit_role: { DDLForbidTruncate: true } } as any;
    expect(isRuleSetCompatibleWithDBType(legacySQL, DBType.MySQL)).toBe(true);
    expect(isRuleSetCompatibleWithDBType(legacySQL, DBType.PostgreSQL)).toBe(true);
    expect(isRuleSetCompatibleWithDBType(legacySQL, DBType.MongoDB)).toBe(false);
  });

  it('rejects explicit dialect metadata that disagrees with rule content', () => {
    expect(
      isRuleSetCompatibleWithDBType(
        { dialect: 'mysql', audit_role: { MongoForbidMultiWrite: true } } as any,
        DBType.MySQL,
      ),
    ).toBe(false);
  });

  it('identifies and isolates Redis and Doris legacy rule sets', () => {
    const redisRuleSet = {
      dialect: '',
      audit_role: { RedisForbidDebug: true },
    } as any;
    expect(isRuleSetCompatibleWithDBType(redisRuleSet, DBType.Redis)).toBe(true);
    expect(isRuleSetCompatibleWithDBType(redisRuleSet, DBType.MongoDB)).toBe(false);
    const dorisWithSQL = {
      dialect: '',
      audit_role: { DorisRequireDistribution: true, DDLForbidTruncate: true },
    } as any;
    expect(isRuleSetCompatibleWithDBType(dorisWithSQL, DBType.Doris)).toBe(true);
    expect(isRuleSetCompatibleWithDBType(dorisWithSQL, DBType.MySQL)).toBe(false);
  });

  it('rejects mixed legacy dialects', () => {
    expect(
      isRuleSetCompatibleWithDBType(
        {
          dialect: '',
          audit_role: { MongoForbidMultiWrite: true, DDLForbidTruncate: true },
        } as any,
        DBType.MongoDB,
      ),
    ).toBe(false);
    expect(
      isRuleSetCompatibleWithDBType(
        {
          dialect: '',
          audit_role: { RedisForbidDebug: true, DorisRequireDistribution: true },
        } as any,
        DBType.Redis,
      ),
    ).toBe(false);
  });
});
