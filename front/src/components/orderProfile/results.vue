<template>
  <a-form v-if="route.params.tp !== 'record'" layout="inline">
    <a-space>
      <template v-if="route.params.tp === 'audit' && status === 1">
        <a-popconfirm
          :title="$t('order.profile.results.roll.tips')"
          @confirm="submit"
        >
          <a-button>{{ $t('order.profile.results.commit.rollback') }}</a-button>
        </a-popconfirm>
      </template>
      <template
        v-else-if="
          route.params.tp === 'common' && (status === 0 || status === 4)
        "
      >
        <a-popconfirm
          :title="$t('order.profile.results.recommit.tips')"
          @confirm="recommit"
        >
          <a-button>{{ $t('order.profile.results.commit.recommit') }}</a-button>
        </a-popconfirm>
      </template>
    </a-space>
  </a-form>
  <br />
  <a-collapse v-model:activeKey="activeKey" accordion>
    <a-collapse-panel key="1" :header="$t('order.profile.results.result')">
      <c-table :tbl-ref="resultTable" size="small"></c-table>
    </a-collapse-panel>
    <a-collapse-panel key="2" :header="$t('order.profile.results.roll')">
      <c-table :tbl-ref="rollTable" is-all size="small"></c-table>
    </a-collapse-panel>
  </a-collapse>
</template>

<script lang="ts" setup>
  import {
    getOrderResult,
    getOrderRollSQLS,
    userPostOrder,
  } from '@/apis/orderPostApis';
  import { ISource, querySourceList } from '@/apis/source';
  import { onMounted, reactive, ref } from 'vue';
  import { useOrderStore } from '@/store';
  import { useRoute, useRouter } from 'vue-router';
  import { useI18n } from 'vue-i18n';
  import { tableRef } from '../table';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { OrderItem } from '@/types';

  interface page {
    current: number;
    pageSize: number;
  }

  const { t } = useI18n();

  const route = useRoute();

  const props = defineProps<{
    workId: string;
    status: number;
  }>();

  const activeKey = ref('1');

  // 0 mysql / 1 PostgreSQL / 2 mongodb / 3 redis（与后端 model.DBType* 一致）
  const dbType = ref(0);

  const resultTable = reactive<tableRef>({
    col: [
      {
        title: t('common.table.sql'),
        dataIndex: 'sql',
      },
      {
        title: t('common.table.result'),
        dataIndex: 'state',
      },
      {
        title: t('common.table.rows'),
        dataIndex: 'affect_row',
      },
      {
        title: t('common.table.time'),
        dataIndex: 'time',
      },
      {
        title: t('common.table.error'),
        dataIndex: 'error',
      },
    ],
    data: [],
    pageCount: 0,
    // 执行结果表保持原样：不固定列宽、不可拖拽，内容自适应换行
    resizable: false,
    fn: async ({ current, pageSize }: page) => {
      const { data } = await getOrderResult(props.workId, {
        current: current,
        pageSize: pageSize,
      });
      resultTable.data = data.payload.record;
      resultTable.pageCount = data.payload.count;
    },
  });

  const rollTable = reactive<tableRef>({
    col: [
      {
        title: t('common.table.sql'),
        dataIndex: 'sql',
      },
    ],
    data: [],
    pageCount: 0,
    resizable: false,
  });

  const router = useRouter();

  const orderStore = useOrderStore();

  const currentRolling = async (vl: number) => {
    const { data } = await getOrderRollSQLS(props.workId, vl);
    rollTable.data = data.payload.sql;
    rollTable.pageCount = data.payload.count;
  };

  // 工单表里没有 db_type：按工单类型查一次数据源列表（dml/ddl 都是权限过滤后的），
  // 供「提交换回滚」决定怎么拼 SQL、「重新提交」决定跳转时带什么类型。
  const loadSourceType = async () => {
    try {
      const order = orderStore.order;
      const { data } = await querySourceList(order.type === 1 ? 'dml' : 'ddl');
      const hit = (data.payload as ISource[] | null)?.find(
        (i) => i.source_id === order.source_id,
      );
      dbType.value = hit?.db_type ?? 0;
    } catch {
      dbType.value = 0; // 取不到就按 SQL 处理（与改动前一致）
    }
  };

  const submit = async () => {
    const statements = rollTable.data.map((item) => item.sql);
    if (statements.length === 0 || statements.join('') === '') {
      message.warning(t('order.roll.tips'));
      return;
    }
    const wrapper = Object.assign({}, orderStore.order);
    wrapper.delay = 'none';
    // MongoDB 一张工单只承载一条命令：回滚语句可能有多条（如 createIndexes 每个索引一条），
    // 逐条提交；用 \n 拼成一条会被提交时的硬保底当成非法 JSON 拒掉。
    if (dbType.value === 2) {
      for (const sql of statements) {
        if (sql === '') continue;
        await userPostOrder(Object.assign({}, wrapper, { sql }) as OrderItem);
      }
      router.go(-1);
      return;
    }
    wrapper.sql = statements.join('\n');
    await userPostOrder(wrapper as OrderItem);
    router.go(-1);
  };

  const recommit = () => {
    router.push({
      name: 'apply/order',
      query: {
        type: orderStore.order.type,
        idc: orderStore.order.idc,
        source: orderStore.order.source,
        source_id: orderStore.order.source_id,
        // 不带 db_type 会被申请页当成 MySQL：Mongo 提示条与集合名提取都不生效
        db_type: dbType.value,
        remark: 'true',
      },
    });
  };

  onMounted(() => {
    currentRolling(1);
    loadSourceType();
  });
</script>

<style></style>
