import i18n from '@/lang';

interface Rule {
  desc: string;
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
    desc: t('DDLCheckTableComment'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDlCheckColumnComment',
    desc: t('DDlCheckColumnComment'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckColumnNullable',
    desc: t('DDLCheckColumnNullable'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckColumnDefault',
    desc: t('DDLCheckColumnDefault'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLCheckFloatDouble',
    desc: t('DDLCheckFloatDouble'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoincrementInit',
    desc: t('DDLEnableAutoincrementInit'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnablePrimaryKey',
    desc: t('DDLEnablePrimaryKey'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLPrimaryKeyMust',
    desc: t('DDLPrimaryKeyMust'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoIncrement',
    desc: t('DDLEnableAutoIncrement'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAutoincrementUnsigned',
    desc: t('DDLEnableAutoincrementUnsigned'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLIndexNameSpec',
    desc: t('DDLIndexNameSpec'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'CheckIdentifier',
    desc: t('CheckIdentifier'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableAcrossDBRename',
    desc: t('DDLEnableAcrossDBRename'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableDropTable',
    desc: t('DDLEnableDropTable'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableDropDatabase',
    desc: t('DDLEnableDropDatabase'),
    type: 'DDL',
    tp: 0,
  },
  {
    // 自研规则（engine/internal/customrules）的开关，作为新增自定义规则的接线示例
    name: 'DDLForbidTruncate',
    desc: t('DDLForbidTruncate'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableForeignKey',
    desc: t('DDLEnableForeignKey'),
    type: 'DDL',
    tp: 0,
  },
  {
    // 表名前缀：与「表名最大长度」合成同一条命名规则（tp=2 为文本输入）
    name: 'DDLTablePrefix',
    desc: t('DDLTablePrefix'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'DDLAllowPRINotInt',
    desc: t('DDLAllowPRINotInt'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLEnableNullIndexName',
    desc: t('DDLEnableNullIndexName'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLMultiToCommit',
    desc: t('DDLMultiToCommit'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowMultiAlter',
    desc: t('DDLAllowMultiAlter'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowColumnType',
    desc: t('DDLAllowColumnType'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'DDLAllowChangeColumnPosition',
    desc: t('DDLAllowChangeColumnPosition'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowCreateView',
    desc: t('AllowCreateView'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowCreatePartition',
    desc: t('AllowCreatePartition'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'AllowSpecialType',
    desc: t('AllowSpecialType'),
    type: 'DDL',
    tp: 0,
  },
  {
    name: 'SupportCollation',
    desc: t('SupportCollation'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'SupportCharset',
    desc: t('SupportCharset'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'MustHaveColumns',
    desc: t('MustHaveColumns'),
    type: 'DDL',
    tp: 2,
  },
  {
    name: 'DDLMaxKeyParts',
    desc: t('DDLMaxKeyParts'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DDLMaxKey',
    desc: t('DDLMaxKey'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'MaxDDLAffectRows',
    desc: t('MaxDDLAffectRows'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DDLMaxCharLength',
    desc: t('DDLMaxCharLength'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'MaxTableNameLen',
    desc: t('MaxTableNameLen'),
    type: 'DDL',
    tp: 1,
  },
  {
    name: 'DMLMaxInsertRows',
    desc: t('DMLMaxInsertRows'),
    type: 'DML',
    tp: 1,
  },
  {
    name: 'DMLWhereExprValueIsNull',
    desc: t('DMLWhereExprValueIsNull'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLAllowLimitSTMT',
    desc: t('DMLAllowLimitSTMT'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLAllowInsertNull',
    desc: t('DMLAllowInsertNull'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DDLImplicitTypeConversion',
    desc: t('DDLImplicitTypeConversion'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLInsertColumns',
    desc: t('DMLInsertColumns'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLInsertMustExplicitly',
    desc: t('DMLInsertMustExplicitly'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLWhere',
    desc: t('DMLWhere'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLOrder',
    desc: t('DMLOrder'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'DMLSelect',
    desc: t('DMLSelect'),
    type: 'DML',
    tp: 0,
  },
  {
    name: 'MaxAffectRows',
    desc: t('MaxAffectRows'),
    type: 'DML',
    tp: 1,
  },
  {
    name: 'DMLTransaction',
    desc: t('DMLTransaction'),
    type: 'DML',
    tp: 0,
  },

  {
    name: 'IsOSC',
    desc: t('IsOSC'),
    type: 'Online-DDL',
    tp: 0,
  },
  {
    name: 'OscSize',
    desc: t('OscSize'),
    type: 'Online-DDL',
    tp: 1,
  },
  {
    name: 'OSCExpr',
    desc: t('OSCExpr'),
    type: 'Online-DDL',
    tp: 3,
  },
  // --- MongoDB 变更规则 ---
  // 与 SQL 规则同构：开关 + 级别（级别走 RuleLevel，默认 error）
  {
    name: 'MongoForbidEmptyFilter',
    desc: t('MongoForbidEmptyFilter'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidDangerous',
    desc: t('MongoForbidDangerous'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidWhere',
    desc: t('MongoForbidWhere'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidDropCollection',
    desc: t('MongoForbidDropCollection'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidSystemCollection',
    desc: t('MongoForbidSystemCollection'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidAdminCommand',
    desc: t('MongoForbidAdminCommand'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidImmutableId',
    desc: t('MongoForbidImmutableId'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidCappedConvert',
    desc: t('MongoForbidCappedConvert'),
    level: 'error',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoForbidCollMod',
    desc: t('MongoForbidCollMod'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoIndexKeyLimit',
    desc: t('MongoIndexKeyLimit'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoIndexNameSpec',
    desc: t('MongoIndexNameSpec'),
    level: 'warn',
    type: 'Mongo',
    tp: 2,
  },
  {
    name: 'MongoCollectionPrefix',
    desc: t('MongoCollectionPrefix'),
    level: 'warn',
    type: 'Mongo',
    tp: 2,
  },
  {
    name: 'MongoMaxCollectionNameLen',
    desc: t('MongoMaxCollectionNameLen'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoRegexUnanchored',
    desc: t('MongoRegexUnanchored'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoNegationOperator',
    desc: t('MongoNegationOperator'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoOrClause',
    desc: t('MongoOrClause'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoLargeInList',
    desc: t('MongoLargeInList'),
    level: 'warn',
    type: 'Mongo',
    tp: 1,
  },
  {
    name: 'MongoQueryForbidNoFilter',
    desc: t('MongoQueryForbidNoFilter'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoQueryForbidLookup',
    desc: t('MongoQueryForbidLookup'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoQueryForbidNoLimit',
    desc: t('MongoQueryForbidNoLimit'),
    level: 'warn',
    type: 'Mongo',
    tp: 0,
  },
  {
    name: 'MongoMaxAffectRows',
    desc: t('MongoMaxAffectRows'),
    level: 'error',
    type: 'Mongo',
    tp: 1,
  },
];

export { rule, Rule };
