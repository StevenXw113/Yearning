import { COMMON_URI, request } from '@/config/request';

export function getRecordAxis() {
  return request.get(`${COMMON_URI}/record/axis`);
}

// ---- 归档查询（后端 ArchiveDays 归档 cron 迁入 *_archive 表的数据）----

export interface ArchiveOrder {
  work_id: string;
  username: string;
  real_name?: string;
  status: number;
  type: number;
  source: string;
  data_base: string;
  date: string;
  execute_time?: string;
  text?: string;
  delay?: string;
}

export interface ArchiveListResult {
  total: number;
  page: number;
  size: number;
  payload: ArchiveOrder[];
}

export interface ArchiveDetail {
  order: ArchiveOrder;
  records: {
    sql: string;
    state: string;
    affect_row: number;
    time: string;
    error?: string;
  }[];
  rollbacks: { sql: string }[];
  comments: { username: string; content: string; time: string }[];
  details: { username: string; time: string; action: string }[];
}

export function getArchiveList(params: {
  page: number;
  size: number;
  date_from?: string;
  date_to?: string;
  work_id?: string;
}) {
  return request.get<{
    payload: ArchiveListResult;
    code: number;
    msg?: string;
  }>(`${COMMON_URI}/record/archive`, { params });
}

export function getArchiveDetail(work_id: string) {
  return request.get<{ payload: ArchiveDetail; code: number; msg?: string }>(
    `${COMMON_URI}/record/archive_detail`,
    { params: { work_id } },
  );
}
