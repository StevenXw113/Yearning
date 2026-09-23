<template>
  <div ref="container">
    <a-card>
      <order-table-search
        ref="search"
        @search="
          (exp) => {
            tblRef.expr = exp;
            tbl.manual(true);
          }
        "
      >
      </order-table-search>
      <c-table
        ref="tbl"
        :tbl-ref="tblRef"
        :size="props.size"
      >
        <template #bodyCell="{ column, text, record }">
          <template v-if="column.dataIndex === 'type'">
            <span>{{ text === 0 ? 'DDL' : 'DML' }}</span>
          </template>
          <template v-if="column.dataIndex === 'assigned'">
            <a-tag v-for="i in (text || '').split(',').filter(Boolean)" :key="i">{{
              i
            }}</a-tag>
          </template>
          <template v-if="column.dataIndex === 'delay'">{{
            text === 'none'
              ? $t('order.table.delay')
              : text === 'manual'
              ? $t('order.exec.manual')
              : text
          }}</template>
          <template v-if="column.dataIndex === 'status'">
            <state-tags :state="text"></state-tags>
          </template>
          <template v-if="column.dataIndex === 'action'">
            <a-space v-if="!record.isProject">
              <a-button type="primary" size="small" @click="profile(record)"
                >{{ $t('common.profile') }}
              </a-button>
              <a-button
                v-if="
                  !props.disabledBtn &&
                  record.status === OrderState.PROCESS &&
                  record.delay !== 'none'
                "
                size="small"
                @click="delay.openSchedule(record.work_id)"
                >{{ $t('order.delay') }}</a-button
              >
            </a-space>
            <span v-else style="color: #999"
              >{{ record.children?.length || 0 }} 个子工单，点击展开审核</span
            >
          </template>
        </template>
      </c-table>
    </a-card>
    <Profile
      :visible="visible"
      :width="width"
      @close="onClose"
      @close-drawer="() => (visible = false)"
    />
    <Delay ref="delay" />
  </div>
</template>

<script lang="ts" setup>
  import StateTags from './stateTags.vue';
  import OrderTableSearch from './orderTableSearch.vue';
  import { onBeforeRouteUpdate, useRoute } from 'vue-router';
  import { onMounted, reactive, ref } from 'vue';
  import { OrderExpr, OrderParams, checkUri } from '@/apis/orderPostApis';
  import { OrderTableData, OrderState } from '@/types';
  import { useStore } from '@/store';
  import { useI18n } from 'vue-i18n';
  import { tableRef } from '@/components/table';
  import type { TableColumnsType } from 'ant-design-vue';
  import { useElementSize, useWebSocket } from '@vueuse/core';
  import { checkSchema } from '@/lib';
  import Profile from '@/components/orderProfile/index.vue';
  import Delay from './delay.vue';
  import { ISource, querySourceList } from '@/apis/source';

  interface propsAttr {
    size?: string;
    disabledBtn?: boolean;
  }

  const props = withDefaults(defineProps<propsAttr>(), {
    size: 'default',
    disabledBtn: false,
  });

  const search = ref();

  const delay = ref();

  const { t } = useI18n();

  const container = ref();

  const route = useRoute();

  const visible = ref<boolean>(false);

  const { width } = useElementSize(container);

  const store = useStore();

  const tbl = ref();

  const isAudit = ref('');

  // 项目级工单：同批次(batch_id)的子工单折叠为一个项目行，展开后逐条审核；
  // 普通单工单（batch_id 为空）保持原样。children 由 a-table 自动渲染成可展开的树。
  const groupByProject = (rows: OrderTableData[]): any[] => {
    const projects = new Map<string, any>();
    const out: any[] = [];
    for (const r of rows || []) {
      const bid = r.batch_id;
      if (!bid) {
        out.push({ ...r, key: r.work_id });
        continue;
      }
      let p = projects.get(bid);
      if (!p) {
        p = {
          key: 'batch-' + bid,
          isProject: true,
          batch_id: bid,
          work_id: bid,
          text: r.text,
          username: r.username,
          real_name: r.real_name,
          type: r.type,
          date: r.date,
          status: r.status,
          assigned: '',
          delay: '',
          children: [],
        };
        projects.set(bid, p);
        out.push(p);
      }
      // 只要还有子工单待审，项目行整体显示为「审核中」
      if (r.status === OrderState.AUDIT) p.status = OrderState.AUDIT;
      p.children.push({ ...r, key: r.work_id });
    }
    projects.forEach((p) => (p.text = `${p.text}（共 ${p.children.length} 条）`));
    return out;
  };

  // 列宽可拖拽由 c-table 统一开启（见 components/table/table.vue），这里只定义列与初始宽度
  const cols: TableColumnsType = [
    {
      title: t('common.table.work_id'),
      dataIndex: 'work_id',
      width: 110,
      fixed: 'left', // 列多需要横向滚动，编号固定在左侧便于对照
    },
    {
      title: t('common.table.source'),
      dataIndex: 'source',
      width: 130,
      filters: [] as any[], // 选项来自 querySourceList('all')，见 onMounted
      filterMultiple: false,
    },
    {
      title: t('common.table.remark'),
      dataIndex: 'text',
      width: 220,
      ellipsis: true,
    },
    {
      title: t('common.table.type'),
      dataIndex: 'type',
      width: 90,
      filters: [
        { text: 'DDL', value: 0 },
        { text: 'DML', value: 1 },
      ],
      filterMultiple: false,
    },
    {
      title: t('common.table.post.time'),
      dataIndex: 'date',
      width: 170,
    },
    {
      title: t('common.table.post.user'),
      dataIndex: 'username',
      width: 110,
    },
    {
      title: t('common.table.post.real_name'),
      dataIndex: 'real_name',
      width: 110,
    },
    {
      title: t('order.profile.timing'),
      dataIndex: 'delay',
      width: 140,
    },
    {
      title: t('order.profile.auditor'),
      dataIndex: 'assigned',
      width: 120,
    },
    {
      title: t('common.table.state'),
      dataIndex: 'status',
      width: 110,
      filters: [
        { text: t('order.state.audit'), value: OrderState.AUDIT },
        { text: t('order.state.success'), value: OrderState.SUCCESS },
        { text: t('order.state.reject'), value: OrderState.REJECT },
        { text: t('order.state.process'), value: OrderState.PROCESS },
        { text: t('order.undo'), value: OrderState.Undo },
      ],
      filterMultiple: false,
    },
    {
      title: t('common.action'),
      dataIndex: 'action',
      width: 210,
      fixed: 'right', // 详情/延迟操作固定在右侧，横向滚动时始终可点
    },
  ];

  const tblRef = reactive<tableRef>({
    col: cols,
    data: [] as OrderTableData[],
    pageCount: 0,
    // 工单列表：固定列宽 + 可拖拽，超长内容截断悬停看全文
    resizable: true,
    defaultPageSize: 20,
    expr: {
      status: 8,
      type: 2,
      text: '',
      username: '',
    } as OrderExpr,
    isloop: true,
    websocket: useWebSocket(
      `${checkSchema()}${checkUri(route.params.tp as string)}`,
      {
        autoReconnect: {
          retries: 3,
        },
        protocols: [store.state.user.account.token],
        onMessage: (e, event) => {
          let payload = JSON.parse(event.data);
          tblRef.data = groupByProject(payload.payload.data);
          tblRef.pageCount = payload.payload.page;
        },
      }
    ),
    fn: async (params: any) => {
      params.expr = applyFilters(tblRef.expr, params.filters);
      tblRef.expr = params.expr;
      tblRef.websocket?.send(JSON.stringify(params));
    },
  });

  // 表头筛选翻译成列表查询已有的 expr 字段（type / status / source），后端按白名单处理
  const applyFilters = (expr: any, filters: any) => {
    const next = { ...(expr || {}) };
    if (filters) {
      if ('type' in filters) next.type = filters.type ? filters.type[0] : 2; // null = 取消筛选
      if ('status' in filters) next.status = filters.status ? filters.status[0] : 8;
      if ('source' in filters) next.source = filters.source ? filters.source[0] : '';
    }
    return next;
  };

  const profile = (record: OrderTableData) => {
    store.commit('order/ORDER_STORE', record);
    visible.value = true;
  };

  const onClose = () => {
    visible.value = false;
  };

  onBeforeRouteUpdate((to) => {
    isAudit.value = to.params.tp as string;
    tbl.value.manual();
  });

  onMounted(async () => {
    isAudit.value = route.params.tp as string;
    // 数据源筛选项：tp=all 返回全部数据源
    const { data } = await querySourceList('all');
    const col = (tblRef.col as any[]).find((c) => c.dataIndex === 'source');
    if (col) {
      col.filters = (data.payload as ISource[]).map((s) => ({
        text: s.source,
        value: s.source,
      }));
    }
  });
</script>

<style scoped>
/* 列多且都是短内容，换行会让行高翻倍；统一不换行，超出宽度时表格内部横向滚动 */
:deep(.ant-table-thead th),
:deep(.ant-table-tbody td) {
  white-space: nowrap;
}

/* 筛选图标紧贴标题：标题默认 flex:1 会撑满整列，把图标顶到列的最右边 */
:deep(.ant-table-column-title) {
  flex: none;
}
:deep(.ant-table-filter-column) {
  justify-content: flex-start;
}
:deep(.ant-table-filter-trigger) {
  margin-left: 4px;
}
</style>
