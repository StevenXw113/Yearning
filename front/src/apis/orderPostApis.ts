import { request, COMMON_URI, Res } from '@/config/request';
import { OrderItem, OrderTableData, SQLTesting } from '@/types';
import { Dayjs } from 'dayjs';

export type RangeValue = [Dayjs, Dayjs];

export interface Comment {
  username?: string;
  time?: string;
  content: string;
  work_id: string;
}

export interface SQLTestParams {
  source_id: string;
  data_base: string;
  sql: string;
  kind: number;
}

export interface SQLAuditOrder {
  work_id: string;
  source_id?: string;
  flag?: number;
  tp?: string;
  text?: string;
}

export interface OrderExpr {
  type: number;
  status: number;
  text: string;
  picker?: RangeValue | string[];
  username: string;
  work_id?: string;
  order?: string;
  source?: string;
}

export interface OrderParams {
  find: OrderExpr;
  page: number;
}

export interface OrderTableResp {
  data: OrderTableData[];
  page: number;
}

export interface Reject {
  work_id: string;
  content: string;
}

export function checkUri(tp: string) {
  switch (tp) {
    case 'audit':
      return `${COMMON_URI}/audit/order/list`;
    case 'common':
      return `${COMMON_URI}/common/list`;
    case 'record':
      return `${COMMON_URI}/record/list`;
    default:
      return '';
  }
}

export function checkSQLS(params: SQLTestParams) {
  return request.put<Res<SQLTesting[]>>(`${COMMON_URI}/fetch/test`, params);
}

export function mergeDDLSTMT(sql: string) {
  return request.put(`${COMMON_URI}/fetch/merge`, { sqls: sql });
}

export function getNextOrderState(args: SQLAuditOrder) {
  return request.post(`${COMMON_URI}/audit/order/state`, args);
}

export function scheduledChange(work_id: string, delay: string) {
  return request.post(`${COMMON_URI}/audit/order/scheduled`, {
    work_id,
    delay,
  });
}

// 人工执行：把已通过、停在「等待执行」(status=5) 的工单落到引擎执行
export function executeOrder(work_id: string) {
  return request.post(`${COMMON_URI}/audit/order/state`, {
    tp: 'execute',
    work_id,
  });
}

// 重试预览：失败明细 + 将要重跑的剩余 SQL（重试弹窗的调试视图）
export function retryPreview(work_id: string) {
  return request.get<Res<RetryPreview>>(
    `${COMMON_URI}/audit/order/retry_preview`,
    { params: { work_id } },
  );
}

// 失败工单重试：只重执未成功执行过的语句，不需要新建工单
export function retryOrder(work_id: string) {
  return request.post(`${COMMON_URI}/audit/order/state`, {
    tp: 'retry',
    work_id,
  });
}

// 跳过失败语句继续执行后续语句（人工已在库上处理完失败部分的数据订正场景）
export function skipAndContinue(work_id: string) {
  return request.post(`${COMMON_URI}/audit/order/state`, {
    tp: 'skip',
    work_id,
  });
}

// 手动标记完成：SQL 在平台外人工处理（如引擎不支持的特殊库）后回填状态，text 为处理说明
export function markOrderFinished(work_id: string, text: string) {
  return request.post(`${COMMON_URI}/audit/order/state`, {
    tp: 'mark',
    work_id,
    text,
  });
}

export function changeOrderStateUndo(args: SQLAuditOrder) {
  return request.post(`${COMMON_URI}/audit/order/state`, args);
}

export function userPostOrder(args: OrderItem) {
  return request.post(`${COMMON_URI}/common/post`, args);
}

// 项目级工单（批量提交）：一次提交多条明细，每条明细一个子工单，共享批次号
export interface BatchOrderItem {
  source_id: string;
  data_base: string;
  file?: string;
  sql: string;
  // 目标表/集合：MongoDB 由前端从命令里提取集合名，其余引擎可留空
  table?: string;
}

export interface RetryPreview {
  remaining: string;
  // 「跳过并继续」视图：失败语句视为已人工处理后，其后将执行的语句
  remaining_skip: string;
  skip_available: boolean;
  executed: { sql: string; state: string; affect_row: number }[];
  failed: { sql: string; state: string; error: string }[];
  error?: string;
}

export interface BatchOrderParams {
  type: number;
  // 工单级事务模式（1=开启）：整批统一，作用于每个子工单
  tx_mode?: number;
  backup: number;
  delay: string;
  text: string;
  items: BatchOrderItem[];
}

export interface BatchExecuteResult {
  work_id: string;
  source: string;
  data_base: string;
  ok: boolean;
  msg: string;
}

export function userPostBatchOrder(args: BatchOrderParams) {
  return request.post<Res<string>>(`${COMMON_URI}/common/batch`, args);
}

export function getBatchOrders(batch_id: string) {
  return request.get<Res<any[]>>(`${COMMON_URI}/common/batch`, {
    params: { batch_id },
  });
}

export function executeBatchOrder(batch_id: string) {
  return request.post<Res<BatchExecuteResult[]>>(
    `${COMMON_URI}/audit/order/batch`,
    { batch_id },
  );
}

export function getOrderList(args: OrderParams, tp: string) {
  return request.put(checkUri(tp), args);
}

export function getOrderResult(
  work_id: string,
  page: { current: number; pageSize: number },
) {
  return request.get(`${COMMON_URI}/fetch/detail`, {
    params: {
      work_id: work_id,
      page: page.current,
      page_size: page.pageSize,
    },
  });
}

export function getOrderRollSQLS(work_id: string, page: number) {
  return request.get(`${COMMON_URI}/fetch/roll`, {
    params: {
      work_id: work_id,
      page: page,
    },
  });
}

export function postOrderComment(args: Comment) {
  return request.post(`${COMMON_URI}/fetch/comment`, args);
}
