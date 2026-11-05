import { request, COMMON_URI, Res } from '@/config/request';

export interface Source {
  idc: string;
  source: string;
  ip: string;
  port: number;
  username: string;
  password: string;
  is_query: number;
  flow_id: number;
  source_id: string;
  exclude_db_list: string;
  insulate_word_list: string;
  principal: string;
  ca_file: string;
  cert: string;
  key_file: string;
  rule_id: number;
  db_type: number;
  // 列表接口返回的自增主键（新增/编辑请求不带）
  id?: number;
  // MongoDB 认证库（authSource），空 = 默认 admin；账号建在业务库上时必填
  auth_database: string;
  // Redis：查询页是否允许诊断命令（SLOWLOG/LATENCY/DUMP/COMMAND/INFO 全量），默认 false
  allow_diag_commands?: boolean;
}

// 数据源类型：value 对应后端 db_type，port 为该类型的默认端口
export const DB_TYPES = [
  { value: 0, label: 'MySQL', port: 3306 },
  { value: 1, label: 'PostgreSQL', port: 5432 },
  { value: 2, label: 'MongoDB', port: 27017 },
  { value: 3, label: 'Redis', port: 6379 },
  { value: 4, label: 'Doris', port: 9030 },
] as const;

export const dbTypeLabel = (value: number) =>
  DB_TYPES.find((i) => i.value === Number(value))?.label ?? String(value);

export const dbTypePort = (value: number, fallback: number) =>
  DB_TYPES.find((i) => i.value === Number(value))?.port ?? fallback;

export interface DBParams {
  page: number;
  find: DBExpr;
}

export interface DBExpr {
  idc: string;
  source: string;
  ip: string;
  is_query: number;
}

export interface DBResp {
  data: Source[];
  page: number;
}

export interface RequestDB {
  tp: string;
  db: Source[] | Source;
  encrypt?: boolean;
}

export function getSourceList(args: DBParams) {
  return request.put<Res<DBResp>>(`${COMMON_URI}/manage/db`, args);
}

export function deleteSource(id: string) {
  return request.delete(`${COMMON_URI}/manage/db?source_id=${id}`);
}

export function createSource(args: RequestDB) {
  return request.post(`${COMMON_URI}/manage/db`, args);
}
