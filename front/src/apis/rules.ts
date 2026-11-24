import { COMMON_URI, Res, request } from '@/config/request';
import { AxiosPromise } from 'axios';
import { DBType, dbTypeDialect } from './db';

export interface Rules {
  [key: string]: string | number | boolean;
}

export type RuleSetLevel = 'error' | 'warn' | 'observe';

export interface RuleSetSettings {
  production_audit_required?: boolean;
  audit_unavailable?: string;
  execution_timeout_ms?: number;
  max_result_rows?: number;
  max_result_bytes?: number;
  max_affect_rows?: number;
}

export interface RuleSetRule {
  id: string;
  enabled: boolean;
  level: RuleSetLevel;
  params?: Record<string, unknown>;
  reason?: string;
}

export interface RuleSetDefinition {
  schema_version: number;
  dialect: string;
  dialects?: string[];
  scope: string;
  settings: RuleSetSettings;
  rules: RuleSetRule[];
}

export interface RuleManifestRule {
  id: string;
  legacy_id: string;
  name: string;
  desc: string;
  category: string;
  modes: string[];
  param_type: 'none' | 'int' | 'string';
  param_key?: string;
  default_params?: Record<string, unknown>;
  default_level: RuleSetLevel;
  safety_baseline: boolean;
  production_recommended: boolean;
  requires_dba_approval: boolean;
}

export interface RuleManifest {
  schema_version: number;
  dialect: string;
  scope: string;
  rules: RuleManifestRule[];
}

export interface CustomRule {
  id?: number;
  desc: string;
  audit_role: Rules;
  definition?: RuleSetDefinition | null;
  dialect?: string;
  scope?: string;
  // 列表接口追加的引用面字段(SuperRolesList 聚合);新增/更新请求不带
  ref_count?: number;
  ref_sources?: string[];
}

export interface RuleSetExport {
  export_version: number;
  dialect: 'mysql' | 'postgres' | 'mongodb' | 'redis' | 'doris' | 'sql';
  compatible_dialects?: string[];
  scope: string;
  audit_role?: Rules;
  definition?: RuleSetDefinition;
}

export type RuleSetImportPayload = RuleSetDefinition | RuleSetExport;

export function getRulesList(): AxiosPromise<Rules> {
  return request.post(`${COMMON_URI}/manage/roles/global`);
}

export function updateGlobalRules(params: Rules): AxiosPromise {
  return request.post(`${COMMON_URI}/manage/roles/global_updated`, params);
}

export function getCustomRulesList(): AxiosPromise<Res<CustomRule[]>> {
  return request.post(`${COMMON_URI}/manage/roles/list`);
}

export function updateRules(params: CustomRule): AxiosPromise {
  return request.post(`${COMMON_URI}/manage/roles/updated`, params);
}

export function addRules(params: CustomRule): AxiosPromise {
  return request.post(`${COMMON_URI}/manage/roles/add`, params);
}

// 删除规则集：被数据源引用时后端会拒绝并说明是哪些数据源
export function deleteRules(params: { id?: number }): AxiosPromise {
  return request.post(`${COMMON_URI}/manage/roles/delete`, params);
}

export function getRulesManifest(): AxiosPromise<Res<RuleManifest>> {
  return request.get<Res<RuleManifest>>(`${COMMON_URI}/manage/rules-manifest`);
}

export function exportRuleSet(rule_id: number) {
  return request.get<Res<RuleSetExport>>(
    `${COMMON_URI}/manage/rule-sets/export`,
    { params: { rule_id } },
  );
}

export function importRuleSet(
  desc: string,
  payload: RuleSetImportPayload,
  targetDialect?: 'mysql' | 'postgres',
): AxiosPromise<Res<CustomRule>> {
  return request.post(`${COMMON_URI}/manage/rule-sets/import`, payload, {
    params: { desc, target_dialect: targetDialect },
  });
}

// MongoDB v2 公开规则 ID。旧 AuditRole 字段只是兼容层，不能作为对外稳定 ID。
const MONGO_LEGACY_ID_MAP: Record<string, string[]> = {
  MongoForbidEmptyFilter: ['mongo.empty_filter'],
  MongoForbidDangerous: ['mongo.dangerous_command'],
  MongoForbidWhere: ['mongo.where'],
  MongoForbidDropCollection: [
    'mongo.collection.drop',
    'mongo.collection.rename',
  ],
  MongoForbidSystemCollection: ['mongo.system_collection'],
  MongoForbidAdminCommand: ['mongo.account.privilege'],
  MongoForbidImmutableId: ['mongo.immutable_id'],
  MongoForbidUpsert: ['mongo.upsert'],
  MongoForbidMultiWrite: ['mongo.multi_write'],
  MongoForbidCappedConvert: ['mongo.capped_convert'],
  MongoForbidCollMod: ['mongo.coll_mod'],
  MongoIndexKeyLimit: ['mongo.index_key_limit'],
  MongoIndexNameSpec: ['mongo.index_name_spec'],
  MongoCollectionPrefix: ['mongo.collection_prefix'],
  MongoMaxCollectionNameLen: ['mongo.max_collection_name_len'],
  MongoRegexUnanchored: ['mongo.regex_unanchored'],
  MongoNegationOperator: ['mongo.negation_operator'],
  MongoOrClause: ['mongo.or_clause'],
  MongoLargeInList: ['mongo.large_in_list'],
  MongoQueryForbidNoFilter: ['mongo.query.no_filter'],
  MongoQueryForbidLookup: ['mongo.query.no_lookup'],
  MongoQueryForbidNoLimit: ['mongo.query.no_limit'],
};

export const MONGO_DBA_APPROVAL_IDS = [
  'mongo.collection.drop',
  'mongo.collection.rename',
  'mongo.account.privilege',
] as const;

interface MongoRuleMeta {
  id: string;
  legacy: string;
  level: RuleSetLevel;
  paramKey?: string;
  paramType?: 'number' | 'string';
}

const MONGO_RULE_META: MongoRuleMeta[] = [
  {
    id: 'mongo.empty_filter',
    legacy: 'MongoForbidEmptyFilter',
    level: 'error',
  },
  {
    id: 'mongo.dangerous_command',
    legacy: 'MongoForbidDangerous',
    level: 'error',
  },
  { id: 'mongo.where', legacy: 'MongoForbidWhere', level: 'error' },
  {
    id: 'mongo.collection.drop',
    legacy: 'MongoForbidDropCollection',
    level: 'error',
  },
  {
    id: 'mongo.collection.rename',
    legacy: 'MongoForbidDropCollection',
    level: 'error',
  },
  {
    id: 'mongo.system_collection',
    legacy: 'MongoForbidSystemCollection',
    level: 'error',
  },
  {
    id: 'mongo.account.privilege',
    legacy: 'MongoForbidAdminCommand',
    level: 'error',
  },
  {
    id: 'mongo.immutable_id',
    legacy: 'MongoForbidImmutableId',
    level: 'error',
  },
  { id: 'mongo.upsert', legacy: 'MongoForbidUpsert', level: 'error' },
  { id: 'mongo.multi_write', legacy: 'MongoForbidMultiWrite', level: 'error' },
  {
    id: 'mongo.capped_convert',
    legacy: 'MongoForbidCappedConvert',
    level: 'error',
  },
  { id: 'mongo.coll_mod', legacy: 'MongoForbidCollMod', level: 'warn' },
  {
    id: 'mongo.index_key_limit',
    legacy: 'MongoIndexKeyLimit',
    level: 'warn',
    paramKey: 'max_keys',
    paramType: 'number',
  },
  {
    id: 'mongo.index_name_spec',
    legacy: 'MongoIndexNameSpec',
    level: 'warn',
    paramKey: 'pattern',
    paramType: 'string',
  },
  {
    id: 'mongo.collection_prefix',
    legacy: 'MongoCollectionPrefix',
    level: 'warn',
    paramKey: 'prefix',
    paramType: 'string',
  },
  {
    id: 'mongo.max_collection_name_len',
    legacy: 'MongoMaxCollectionNameLen',
    level: 'warn',
    paramKey: 'max_length',
    paramType: 'number',
  },
  {
    id: 'mongo.regex_unanchored',
    legacy: 'MongoRegexUnanchored',
    level: 'warn',
  },
  {
    id: 'mongo.negation_operator',
    legacy: 'MongoNegationOperator',
    level: 'warn',
  },
  { id: 'mongo.or_clause', legacy: 'MongoOrClause', level: 'warn' },
  {
    id: 'mongo.large_in_list',
    legacy: 'MongoLargeInList',
    level: 'warn',
    paramKey: 'max_items',
    paramType: 'number',
  },
  {
    id: 'mongo.query.no_filter',
    legacy: 'MongoQueryForbidNoFilter',
    level: 'warn',
  },
  {
    id: 'mongo.query.no_lookup',
    legacy: 'MongoQueryForbidLookup',
    level: 'warn',
  },
  {
    id: 'mongo.query.no_limit',
    legacy: 'MongoQueryForbidNoLimit',
    level: 'warn',
  },
];

const isConfiguredValue = (value: unknown): boolean => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') return value.trim() !== '';
  return false;
};

const mongoLevels = (audit: Rules): Record<string, string> => {
  const value = (audit as { RuleLevel?: Record<string, string> }).RuleLevel;
  return value || {};
};

// 将旧 AuditRole 展示为稳定 mongo.* ID。DBA 硬底线是安全边界，可显式要求始终展示。
export function mongoRuleIdsFromAuditRole(
  audit: Rules,
  includeDBABaseline = false,
): string[] {
  const out = new Set<string>();
  for (const [legacy, ids] of Object.entries(MONGO_LEGACY_ID_MAP)) {
    if (isConfiguredValue((audit as Record<string, unknown>)[legacy])) {
      ids.forEach((id) => out.add(id));
    }
  }
  if (includeDBABaseline) {
    MONGO_DBA_APPROVAL_IDS.forEach((id) => out.add(id));
  }
  return [...out];
}

// 前端在保存 MongoDB 规则集时生成 v2 Definition；后端仍会重新校验并双写兼容层。
export function buildMongoRuleSetDefinition(audit: Rules): RuleSetDefinition {
  const levels = mongoLevels(audit);
  const rules: RuleSetRule[] = [];
  for (const meta of MONGO_RULE_META) {
    const raw = (audit as Record<string, unknown>)[meta.legacy];
    const isDBABaseline = (
      MONGO_DBA_APPROVAL_IDS as readonly string[]
    ).includes(meta.id);
    // dropCollection / renameCollection / 账号权限不受普通开关影响，必须进入 DBA 特批流。
    const enabled = isDBABaseline || isConfiguredValue(raw);
    if (!enabled) continue;

    const rule: RuleSetRule = {
      id: meta.id,
      enabled: true,
      level: (levels[meta.legacy] as RuleSetLevel) || meta.level,
    };
    if (meta.paramKey && meta.paramType) {
      const value = raw as string | number | undefined;
      if (
        meta.paramType === 'number' &&
        typeof value === 'number' &&
        value > 0
      ) {
        rule.params = { [meta.paramKey]: value };
      }
      if (
        meta.paramType === 'string' &&
        typeof value === 'string' &&
        value.trim() !== ''
      ) {
        rule.params = { [meta.paramKey]: value };
      }
    }
    rules.push(rule);
  }
  return {
    schema_version: 2,
    dialect: 'mongodb',
    dialects: ['mongodb'],
    scope: 'data_source',
    settings: {
      max_affect_rows: Number(audit.MongoMaxAffectRows || 0),
    },
    rules,
  };
}

// 编辑既有 MongoDB 规则集时以 Definition 为事实源：只更新 UI 已映射的规则/设置，
// 保留未来规则、未知设置和已存在但被禁用的规则。
export function updateMongoRuleSetDefinition(
  current: RuleSetDefinition | null,
  audit: Rules,
): RuleSetDefinition {
  if (!current) return buildMongoRuleSetDefinition(audit);
  const levels = mongoLevels(audit);
  const base = JSON.parse(JSON.stringify(current)) as RuleSetDefinition;
  base.schema_version = 2;
  base.dialect = 'mongodb';
  base.dialects = ['mongodb'];
  base.scope = 'data_source';
  base.settings = {
    ...base.settings,
    max_affect_rows: Number(audit.MongoMaxAffectRows || 0),
  };

  for (const meta of MONGO_RULE_META) {
    const raw = (audit as Record<string, unknown>)[meta.legacy];
    const isDBABaseline = (
      MONGO_DBA_APPROVAL_IDS as readonly string[]
    ).includes(meta.id);
    const enabled = isDBABaseline || isConfiguredValue(raw);
    const existingIndex = base.rules.findIndex((item) => item.id === meta.id);
    const existing = existingIndex >= 0 ? base.rules[existingIndex] : null;
    const params = { ...(existing?.params || {}) };
    if (meta.paramKey && meta.paramType) {
      if (
        meta.paramType === 'number' &&
        typeof raw === 'number' &&
        raw > 0
      ) {
        params[meta.paramKey] = raw;
      } else if (
        meta.paramType === 'string' &&
        typeof raw === 'string' &&
        raw.trim() !== ''
      ) {
        params[meta.paramKey] = raw;
      } else {
        delete params[meta.paramKey];
      }
    }
    const hasParams = Object.keys(params).length > 0;
    const nextRule: RuleSetRule = {
      id: meta.id,
      enabled,
      level: (levels[meta.legacy] as RuleSetLevel) || meta.level,
      ...(hasParams ? { params } : {}),
      ...(existing?.reason !== undefined ? { reason: existing.reason } : {}),
    };
    if (existingIndex >= 0) {
      base.rules[existingIndex] = nextRule;
    } else if (enabled) {
      base.rules.push(nextRule);
    }
  }
  return base;
}

// 数据源绑定前按方言预过滤。空 dialect 的旧规则集保持兼容；MongoDB 只允许纯 Mongo 规则集。
type LegacyRuleDialect = 'sql' | 'mongodb' | 'redis' | 'doris';

const inferLegacyRuleDialect = (
  audit: Rules,
): LegacyRuleDialect | '' => {
  const detected = new Set<LegacyRuleDialect>();
  const add = (key: string, value: unknown) => {
    if (!isConfiguredValue(value)) return;
    if (key.startsWith('Mongo')) detected.add('mongodb');
    else if (key.startsWith('Redis')) detected.add('redis');
    else if (key.startsWith('Doris')) detected.add('doris');
    else detected.add('sql');
  };
  for (const [key, value] of Object.entries(audit)) {
    if (key === 'RuleLevel') {
      const levels = (audit as { RuleLevel?: Record<string, string> }).RuleLevel;
      for (const [levelKey, levelValue] of Object.entries(levels || {})) {
        add(levelKey, levelValue);
      }
      continue;
    }
    add(key, value);
  }
  const hasSQL = detected.has('sql');
  const hasMongo = detected.has('mongodb');
  const hasRedis = detected.has('redis');
  const hasDoris = detected.has('doris');
  if ((hasMongo && (hasSQL || hasRedis || hasDoris)) || (hasRedis && hasDoris)) {
    return '';
  }
  if (hasMongo) return 'mongodb';
  if (hasRedis) return 'redis';
  if (hasDoris) return 'doris';
  if (hasSQL) return 'sql';
  return '';
};

// 数据源绑定前按方言预过滤。旧 SQL 规则集无法区分 MySQL/PostgreSQL，保留双向兼容；
// MongoDB / Redis / Doris 必须绑定各自专用规则集。
export function isRuleSetCompatibleWithDBType(
  ruleSet: Pick<CustomRule, 'dialect' | 'audit_role' | 'definition'>,
  dbType: number,
): boolean {
  const dialect = ruleSet.dialect || ruleSet.definition?.dialect || '';
  const expected = dbTypeDialect(dbType);
  const inferred = inferLegacyRuleDialect(ruleSet.audit_role || {});
  const inferredCompatible =
    expected === 'mysql' || expected === 'postgres'
      ? inferred === 'sql' || inferred === ''
      : inferred === expected;
  if (dialect) return dialect === expected && inferredCompatible;
  return inferredCompatible;
}

export function ruleSetStableIds(ruleSet: CustomRule): string[] {
  if (ruleSet.definition?.rules?.length) {
    return ruleSet.definition.rules.filter((r) => r.enabled).map((r) => r.id);
  }
  if (ruleSet.dialect === 'mongodb') {
    return mongoRuleIdsFromAuditRole(ruleSet.audit_role, true);
  }
  return [];
}

// ---- 自定义审核规则（用户配置的文本规则，与内置规则并存） ----
export interface CustomRuleItem {
  id?: number;
  name: string;
  desc: string;
  // 正则（Go RE2），对提交的 SQL 文本做匹配
  pattern: string;
  level: 'error' | 'warning';
  enabled: number;
}

export function getCustomRuleList(): AxiosPromise<Res<CustomRuleItem[]>> {
  return request.post(`${COMMON_URI}/manage/roles/custom_list`);
}

export function saveCustomRule(
  params: CustomRuleItem,
): AxiosPromise<Res<unknown>> {
  return request.post(`${COMMON_URI}/manage/roles/custom_save`, params);
}

export function deleteCustomRule(params: {
  id?: number;
}): AxiosPromise<Res<unknown>> {
  return request.post(`${COMMON_URI}/manage/roles/custom_delete`, params);
}

export interface RuleHistory {
  id: number;
  rule_id: number;
  operator: string;
  note: string;
  created_at: string;
}

// rule_id 语义与后端一致：0 = 全局规则，>0 = 规则集 id。
export function getRuleHistory(
  rule_id: number,
): AxiosPromise<Res<RuleHistory[]>> {
  return request.post(`${COMMON_URI}/manage/roles/history`, { rule_id });
}

// 回滚到某条历史记录的变更前状态，返回恢复后的规则集。
export function rollbackRule(id: number): AxiosPromise<Res<CustomRule>> {
  return request.post(`${COMMON_URI}/manage/roles/rollback`, { id });
}

export interface UpstreamStatus {
  current: string;
  latest: string;
  hasUpdate: boolean;
  // 非空 = 本次检测没做成（多为访问 GitHub 超时），不是接口失败
  note?: string;
}

// 只检测上游 Bytebase 是否有新版本；更新需重新同步源码并编译发布。
export function checkUpstreamRules(): AxiosPromise<Res<UpstreamStatus>> {
  return request.post(`${COMMON_URI}/manage/roles/upstream`);
}
