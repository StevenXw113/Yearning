import { COMMON_URI, request, Res } from '@/config/request';
import { LabelInValue } from '@/types';

export interface AutoTask {
  name: string;
  tp: number;
  sourceLabel: LabelInValue;
  data_base: string;
  table: string;
  affect_rows: number;
  status?: number;
  idc: string;
  source?: string;
  source_id?: string;
}

export interface AutoTaskExpr {
  text: string;
  source: string;
  tp: number;
}

export interface AutoTaskParams {
  page: number;
  expr: AutoTaskExpr;
}

export interface AutoTaskResp {
  data: AutoTask[];
  page: number;
}

export function getAutoTaskList(params: AutoTaskParams) {
  return request.put<Res<AutoTaskResp>>(`${COMMON_URI}/manage/task`, params);
}

export function updateAutoTask(tp: string, params: AutoTask) {
  tp === 'curd'
    ? ((params.source_id = params.sourceLabel.value),
      (params.source = params.sourceLabel.label))
    : null;
  return request.post(`${COMMON_URI}/manage/task`, {
    task: params,
    tp: tp,
  });
}

export function deleteAutoTask(id: string) {
  return request.delete(`${COMMON_URI}/manage/task?task_id=${id}`);
}

