import i18n from '@/lang';

export type RuleCategory =
  'statement' | 'table' | 'column' | 'index' | 'naming' | 'system' | 'engine';

interface Rule {
  desc: string;
  // category 分类页签的归属(纯展示维度,不进存储、不影响引擎)
  category: RuleCategory;
  name: string;
  type: string;
  tp: number;
  // level 规则自带的默认级别（引擎缺少配置时用的那一档），只用于把级别下拉显示成
  // 「默认(拦截/提示/观察)」。必须与引擎的 DefaultLevel 一致，由 archguard 护栏核对。
  level?: 'error' | 'warn' | 'observe';
}

const { t } = i18n.global;

const rule: Rule[] = [
  {
    name: 'DDLCheckTableComment',
    category: 'table',

    desc: t('DDLCheckTableComment'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDlCheckColumnComment',
    category: 'column',

    desc: t('DDlCheckColumnComment'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckColumnNullable',
    category: 'column',

    desc: t('DDLCheckColumnNullable'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckColumnDefault',
    category: 'column',

    desc: t('DDLCheckColumnDefault'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckFloatDouble',
    category: 'column',

    desc: t('DDLCheckFloatDouble'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoincrementInit',
    category: 'column',

    desc: t('DDLEnableAutoincrementInit'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnablePrimaryKey',
    category: 'table',

    desc: t('DDLEnablePrimaryKey'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLPrimaryKeyMust',
    category: 'table',

    desc: t('DDLPrimaryKeyMust'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoIncrement',
    category: 'column',

    desc: t('DDLEnableAutoIncrement'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoincrementUnsigned',
    category: 'column',

    desc: t('DDLEnableAutoincrementUnsigned'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLIndexNameSpec',
    category: 'naming',

    desc: t('DDLIndexNameSpec'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'CheckIdentifier',
    category: 'naming',

    desc: t('CheckIdentifier'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAcrossDBRename',
    category: 'table',

    desc: t('DDLEnableAcrossDBRename'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableDropTable',
    category: 'table',

    desc: t('DDLEnableDropTable'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableDropDatabase',
    category: 'table',

    desc: t('DDLEnableDropDatabase'),
    type: 'DDL',
    tp: 0,
  },
  {
    // 自研规则（engine/internal/customrules）的开关，作为新增自定义规则的接线示例
    name: 'DDLForbidTruncate',
    category: 'table',

    desc: t('DDLForbidTruncate'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableForeignKey',
    category: 'table',

    desc: t('DDLEnableForeignKey'),
    type: 'DDL',
    tp: 0,
  },
  {
    // 表名前缀：与「表名最大长度」合成同一条命名规则（tp=2 为文本输入）
    name: 'DDLTablePrefix',
    category: 'naming',

    desc: t('DDLTablePrefix'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'DDLAllowPRINotInt',
    category: 'table',

    desc: t('DDLAllowPRINotInt'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableNullIndexName',
    category: 'index',

    desc: t('DDLEnableNullIndexName'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLMultiToCommit',
    category: 'engine',

    desc: t('DDLMultiToCommit'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowMultiAlter',
    category: 'table',

    desc: t('DDLAllowMultiAlter'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowColumnType',
    category: 'column',

    desc: t('DDLAllowColumnType'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowChangeColumnPosition',
    category: 'column',

    desc: t('DDLAllowChangeColumnPosition'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowCreateView',
    category: 'table',

    desc: t('AllowCreateView'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowCreatePartition',
    category: 'table',

    desc: t('AllowCreatePartition'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowSpecialType',
    category: 'system',

    desc: t('AllowSpecialType'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'SupportCollation',
    category: 'system',

    desc: t('SupportCollation'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'SupportCharset',
    category: 'system',

    desc: t('SupportCharset'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'MustHaveColumns',
    category: 'system',

    desc: t('MustHaveColumns'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'DDLMaxKeyParts',
    category: 'index',

    desc: t('DDLMaxKeyParts'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DDLMaxKey',
    category: 'index',

    desc: t('DDLMaxKey'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'MaxDDLAffectRows',
    category: 'engine',

    desc: t('MaxDDLAffectRows'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DDLMaxCharLength',
    category: 'column',

    desc: t('DDLMaxCharLength'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'MaxTableNameLen',
    category: 'naming',

    desc: t('MaxTableNameLen'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DMLMaxInsertRows',
    category: 'engine',

    desc: t('DMLMaxInsertRows'),
    type: 'DML',
    tp: 1,
  },
  {
    name: 'DMLWhereExprValueIsNull',
    category: 'statement',

    desc: t('DMLWhereExprValueIsNull'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLAllowLimitSTMT',
    category: 'statement',

    desc: t('DMLAllowLimitSTMT'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLAllowInsertNull',
    category: 'statement',

    desc: t('DMLAllowInsertNull'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DDLImplicitTypeConversion',
    category: 'statement',

    desc: t('DDLImplicitTypeConversion'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLInsertColumns',
    category: 'statement',

    desc: t('DMLInsertColumns'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLInsertMustExplicitly',
    category: 'statement',

    desc: t('DMLInsertMustExplicitly'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLWhere',
    category: 'statement',

    desc: t('DMLWhere'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLOrder',
    category: 'statement',

    desc: t('DMLOrder'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLSelect',
    category: 'statement',

    desc: t('DMLSelect'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'MaxAffectRows',
    category: 'engine',

    desc: t('MaxAffectRows'),
    type: 'DML',
    tp: 1,
  },
  {
    name: 'DMLTransaction',
    category: 'engine',

    desc: t('DMLTransaction'),
    type: 'DML',
    tp: 0,
  },

  {
    name: 'IsOSC',
    category: 'engine',

    desc: t('IsOSC'),
    type: 'Online-DDL',
    tp: 0,
  },
  {
    name: 'OscSize',
    category: 'engine',

    desc: t('OscSize'),
    type: 'Online-DDL',
    tp: 1,
  },
  {
    name: 'OSCExpr',
    category: 'engine',

    desc: t('OSCExpr'),
    type: 'Online-DDL',
    tp: 3,
  },
  // --- MongoDB 变更规则 ---
  // 与 SQL 规则同构：开关 + 级别（级别走 RuleLevel，默认 error）
  {
    name: 'MongoForbidEmptyFilter',
    category: 'statement',

    desc: t('MongoForbidEmptyFilter'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidDangerous',
    category: 'statement',

    desc: t('MongoForbidDangerous'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidWhere',
    category: 'statement',

    desc: t('MongoForbidWhere'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidDropCollection',
    category: 'table',

    desc: t('MongoForbidDropCollection'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidSystemCollection',
    category: 'table',

    desc: t('MongoForbidSystemCollection'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidAdminCommand',
    category: 'statement',

    desc: t('MongoForbidAdminCommand'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidImmutableId',
    category: 'table',

    desc: t('MongoForbidImmutableId'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidCappedConvert',
    category: 'table',

    desc: t('MongoForbidCappedConvert'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidCollMod',
    category: 'table',

    desc: t('MongoForbidCollMod'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoIndexKeyLimit',
    category: 'index',

    desc: t('MongoIndexKeyLimit'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoIndexNameSpec',
    category: 'naming',

    desc: t('MongoIndexNameSpec'),
    level: 'warn',
    type: 'Mongo',
    tp: 2,
  },
  {
    name: 'MongoCollectionPrefix',
    category: 'naming',

    desc: t('MongoCollectionPrefix'),
    level: 'warn',
    type: 'Mongo',
    tp: 2,
  },
  {
    name: 'MongoMaxCollectionNameLen',
    category: 'naming',

    desc: t('MongoMaxCollectionNameLen'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoRegexUnanchored',
    category: 'statement',

    desc: t('MongoRegexUnanchored'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoNegationOperator',
    category: 'statement',

    desc: t('MongoNegationOperator'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoOrClause',
    category: 'statement',

    desc: t('MongoOrClause'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoLargeInList',
    category: 'statement',

    desc: t('MongoLargeInList'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoQueryForbidNoFilter',
    category: 'statement',

    desc: t('MongoQueryForbidNoFilter'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoQueryForbidLookup',
    category: 'statement',

    desc: t('MongoQueryForbidLookup'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoQueryForbidNoLimit',
    category: 'statement',

    desc: t('MongoQueryForbidNoLimit'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoMaxAffectRows',
    category: 'engine',

    desc: t('MongoMaxAffectRows'),
    level: 'error',
    type: 'Mongo',
    tp: 1,
  },
  // --- Redis 命令规则 ---
  // 安全类默认 error；结构/性能类默认只提示（可在级别列调成 error 来强制）
  {
    name: 'RedisForbidDangerous',
    category: 'statement',

    desc: t('RedisForbidDangerous'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidAdmin',
    category: 'statement',

    desc: t('RedisForbidAdmin'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidScript',
    category: 'statement',

    desc: t('RedisForbidScript'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidReplica',
    category: 'statement',

    desc: t('RedisForbidReplica'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidKeysCommand',
    category: 'statement',

    desc: t('RedisForbidKeysCommand'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidRename',
    category: 'system',

    desc: t('RedisForbidRename'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisKeyPrefix',
    category: 'naming',

    desc: t('RedisKeyPrefix'),
    type: 'Redis',
    tp: 2,
  },
  {
    name: 'RedisMaxKeyLen',
    category: 'naming',

    desc: t('RedisMaxKeyLen'),
    type: 'Redis',
    tp: 1,
  },
  {
    name: 'RedisForbidMove',
    category: 'engine',

    desc: t('RedisForbidMove'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisMaxAffectKeys',
    category: 'engine',

    desc: t('RedisMaxAffectKeys'),
    type: 'Redis',
    tp: 1,
  },
  {
    name: 'RedisMaxValueLen',
    category: 'engine',

    desc: t('RedisMaxValueLen'),
    type: 'Redis',
    tp: 1,
  },
  {
    name: 'RedisRequireTTL',
    category: 'system',

    desc: t('RedisRequireTTL'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidPersist',
    category: 'system',

    desc: t('RedisForbidPersist'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisScanRequireMatch',
    category: 'statement',

    desc: t('RedisScanRequireMatch'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'RedisForbidBlocking',
    category: 'statement',

    desc: t('RedisForbidBlocking'),
    type: 'Redis',
    tp: 0,
  },
  {
    name: 'DorisRequireDistribution',
    category: 'table',

    desc: t('DorisRequireDistribution'),
    type: 'Doris',
    tp: 0,
  },
  {
    name: 'DorisBucketLimit',
    category: 'table',

    desc: t('DorisBucketLimit'),
    type: 'Doris',
    tp: 1,
  },
  {
    name: 'DorisReplicationNum',
    category: 'table',

    desc: t('DorisReplicationNum'),
    type: 'Doris',
    tp: 1,
  },
  {
    name: 'DorisKeyModel',
    category: 'table',

    desc: t('DorisKeyModel'),
    type: 'Doris',
    tp: 0,
  },
];

export { rule, Rule };

// 七类页签:顺序即页签顺序,计数由 categoryCounts 运行时算出(设计见 .zcode/superpowers/design.md)。
export const RULE_CATEGORIES: { key: RuleCategory; i18n: string }[] = [
  { key: 'statement', i18n: 'ruleCatStatement' },
  { key: 'table', i18n: 'ruleCatTable' },
  { key: 'column', i18n: 'ruleCatColumn' },
  { key: 'index', i18n: 'ruleCatIndex' },
  { key: 'naming', i18n: 'ruleCatNaming' },
  { key: 'system', i18n: 'ruleCatSystem' },
  { key: 'engine', i18n: 'ruleCatEngine' },
];

export function filterRules(list: Rule[], cat: RuleCategory | 'all'): Rule[] {
  return cat === 'all' ? list : list.filter((r) => r.category === cat);
}

export function categoryCounts(
  list: Rule[],
): Record<RuleCategory | 'all', number> {
  const out = { all: list.length } as Record<RuleCategory | 'all', number>;
  for (const c of RULE_CATEGORIES) out[c.key] = 0;
  for (const r of list) out[r.category] += 1;
  return out;
}
