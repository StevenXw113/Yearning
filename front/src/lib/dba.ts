import { OrderState } from '@/types';

export type DBAApprovalStatus = 'none' | 'pending' | 'approved' | 'rejected';

interface DBAStatusInput {
  requires_dba_approval?: boolean;
  dba_approval_status?: DBAApprovalStatus;
}

interface DBAStatusMeta {
  textKey: string;
  color: 'default' | 'orange' | 'green' | 'red';
}

export function dbaStatusMeta(status?: DBAApprovalStatus): DBAStatusMeta {
  switch (status) {
    case 'pending':
      return { textKey: 'order.dba.pending', color: 'orange' };
    case 'approved':
      return { textKey: 'order.dba.approved', color: 'green' };
    case 'rejected':
      return { textKey: 'order.dba.rejected', color: 'red' };
    default:
      return { textKey: 'order.dba.none', color: 'default' };
  }
}

export function shouldShowDBAStatus(order: DBAStatusInput): boolean {
  return order.requires_dba_approval === true;
}

export function canReviewDBAApproval(
  routeType: string,
  order: DBAStatusInput & { status?: number },
): boolean {
  return (
    routeType === 'audit' &&
    order.requires_dba_approval === true &&
    (order.status === OrderState.AUDIT || order.status === OrderState.WAIT) &&
    (order.dba_approval_status === 'none' ||
      order.dba_approval_status === 'pending')
  );
}
