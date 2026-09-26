import { COMMON_URI, Res, request } from '@/config/request';
import { AxiosPromise } from 'axios';

export interface Rules {
  [key: string]: string | number | boolean;
}

export interface CustomRule {
  id?: number;
  desc: string;
  audit_role: Rules;
}

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

export interface RuleHistory {
  id: number;
  rule_id: number;
  operator: string;
  note: string;
  created_at: string;
}

// rule_id 语义与后端一致：0 = 全局规则，>0 = 规则集 id。
export function getRuleHistory(rule_id: number): AxiosPromise<Res<RuleHistory[]>> {
  return request.post(`${COMMON_URI}/manage/roles/history`, { rule_id });
}

// 回滚到某条历史记录的变更前状态，返回恢复后的规则集。
export function rollbackRule(id: number): AxiosPromise<Res<Rules>> {
  return request.post(`${COMMON_URI}/manage/roles/rollback`, { id });
}

export interface UpstreamStatus {
  current: string;
  latest: string;
  hasUpdate: boolean;
}

// 只检测上游 Bytebase 是否有新版本；更新需重新同步源码并编译发布。
export function checkUpstreamRules(): AxiosPromise<Res<UpstreamStatus>> {
  return request.post(`${COMMON_URI}/manage/roles/upstream`);
}
