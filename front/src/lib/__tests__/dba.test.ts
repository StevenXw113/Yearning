import { describe, expect, it } from 'vitest';
import {
  canReviewDBAApproval,
  dbaStatusMeta,
  shouldShowDBAStatus,
} from '../dba';
import { OrderState } from '@/types';

describe('MongoDB DBA 特批 UI 逻辑', () => {
  it('非特批工单不显示状态，特批工单按状态展示', () => {
    expect(shouldShowDBAStatus({ requires_dba_approval: false })).toBe(false);
    expect(
      shouldShowDBAStatus({
        requires_dba_approval: true,
        dba_approval_status: 'pending',
      }),
    ).toBe(true);
    expect(dbaStatusMeta('pending').textKey).toBe('order.dba.pending');
    expect(dbaStatusMeta('approved').color).toBe('green');
    expect(dbaStatusMeta('rejected').color).toBe('red');
  });

  it('只有审核页未完结特批允许操作', () => {
    const order = {
      requires_dba_approval: true,
      status: OrderState.AUDIT,
      dba_approval_status: 'none' as const,
    };
    expect(canReviewDBAApproval('audit', order)).toBe(true);
    expect(canReviewDBAApproval('common', order)).toBe(false);
    expect(
      canReviewDBAApproval('audit', {
        ...order,
        status: OrderState.WAIT,
        dba_approval_status: 'pending' as const,
      }),
    ).toBe(true);
    expect(
      canReviewDBAApproval('audit', {
        ...order,
        dba_approval_status: 'approved' as const,
      }),
    ).toBe(false);
  });
});
