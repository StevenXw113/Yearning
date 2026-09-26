// 公共引用的type

export interface LoginRespPayload {
  token: string;
  real_name: string;
  rule: string;
  user: string;
  is_record: number;
}

export enum OrderState {
  REJECT = 0,
  SUCCESS,
  AUDIT,
  PROCESS,
  ERROR,
  WAIT,
  Undo,
  Terminate,
}

export enum QueryState {
  AUDIT = 1,
  PROCESS,
  DONE,
  REJECT,
}

export interface OrderTableData {
  work_id: string;
  username: string;
  text: string;
  backup: number;
  date: string;
  real_name: string;
  executor: string;
  status: number;
  type: number;
  delay: string;
  source: string;
  idc: string;
  data_base: string;
  table: string;
  execute_time: string;
  assigned: string;
  current_step: number;
  relevant: [];
  source_id?: string;
  sql?: string;
  file?: string;
  tables?: string[];
  // 项目级工单（批量）批次号：同批次子工单在前端折叠为一个项目行
  batch_id?: string;
  // 前端聚合出的项目行标记（非后端字段）
  isProject?: boolean;
}

export interface OrderItem {
  type: string | number;
  idc: string;
  source: string;
  source_id: string;
  data_base: string;
  table: string;
  tables: string[];
  delay: string;
  text: string;
  backup: number;
  sql?: string;
  // 上传 SQL 文件时记录的文件名，随工单保存便于追溯
  file?: string;
  relevant: string[];
}

export interface SQLTesting {
  status: number;
  level: number;
  error: string;
  sql: string;
  affect_rows: string;
}

export interface AuditorList {
  username: string;
  real_name: string;
}

export interface LabelInValue {
  label: string;
  value: string;
}

export interface Clip {
  title: string;
  desc: string;
}

export interface CommonPage<T> {
  expr: T;
  current: number;
  page_size: number;
}
