import { COMMON_URI, Res, request } from '@/config/request';
import { OrderItem } from '@/types';

export function FetchSQLAdvisor(post: OrderItem, type: string) {
  return request.put<Res<any>>(`${COMMON_URI}/fetch/${type}`, post);
}
