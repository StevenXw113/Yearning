import { reactive, UnwrapRef } from 'vue';
import { OrderItem } from '@/types';
import i18n from '@/lang';
// @ts-ignore
const { t } = i18n.global;
export default function () {
  const col = [
    {
      title: t('common.table.stage'),
      dataIndex: 'status',
      width: 90,
    },
    {
      title: t('common.table.level'),
      dataIndex: 'level',
      width: 100,
    },
    {
      title: t('common.table.error'),
      dataIndex: 'error',
      width: 180,
    },
    {
      title: t('common.table.sql'),
      dataIndex: 'sql',
      width: 340,
    },
    {
      title: t('common.table.max'),
      dataIndex: 'affect_rows',
      width: 120,
    },
  ];

  // 表结构 / 索引详情：仅作为未取到数据时的初始列头；拿到数据后
  // 由 apply/order.vue 按接口真实返回的字段重新生成（见 fieldTitles/indexTitles）
  const tableArch = [
    {
      title: t('order.table.field'),
      dataIndex: 'field',
    },
    {
      title: t('order.table.type'),
      dataIndex: 'type',
    },
    {
      title: t('order.table.isnull'),
      dataIndex: 'null',
    },
    {
      title: t('order.table.default'),
      dataIndex: 'default',
    },
    {
      title: t('order.table.extra'),
      dataIndex: 'comment',
    },
  ];

  const indexArch = [
    {
      title: t('order.table.index'),
      dataIndex: 'IndexName',
    },
    {
      title: t('order.table.isunique'),
      dataIndex: 'NonUnique',
    },
    {
      title: t('order.table.field'),
      dataIndex: 'ColumnName',
    },
  ];

  const orderItems: UnwrapRef<OrderItem> = reactive({
    type: -1,
    idc: '',
    source: '',
    data_base: '',
    table: '',
    text: '',
    delay: '',
    backup: 1,
    sql: '',
    source_id: '',
    relevant: [] as string[],
    work_id: '',
    tables: [],
  });

  return {
    col,
    orderItems,
    tableArch,
    indexArch,
  };
}
