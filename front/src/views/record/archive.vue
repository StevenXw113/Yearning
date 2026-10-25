<template>
  <div style="padding: 0 8px">
    <a-space style="margin-bottom: 12px" wrap>
      <a-input
        v-model:value="searchWorkId"
        :placeholder="$t('record.archive.workIdPh')"
        style="width: 260px"
        allow-clear
        @press-enter="load(1)"
      />
      <a-range-picker v-model:value="dateRange" value-format="YYYY-MM-DD" />
      <a-button type="primary" size="small" @click="load(1)">{{
        $t('common.query')
      }}</a-button>
      <a-button size="small" @click="resetFilters">{{
        $t('common.cancel')
      }}</a-button>
    </a-space>

    <a-table
      size="small"
      :columns="columns"
      :data-source="rows"
      :loading="loading"
      :pagination="pagination"
      row-key="work_id"
      :custom-row="customRow"
    >
      <template #bodyCell="{ column, text }">
        <template v-if="column.key === 'status'">
          <a-tag :color="statusColor(text)">{{ statusText(text) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'type'">
          {{ text === 0 ? 'DDL' : 'DML' }}
        </template>
      </template>
    </a-table>

    <a-drawer
      v-model:open="drawerOpen"
      :title="detailWorkId"
      width="720"
      destroy-on-close
    >
      <a-spin :spinning="detailLoading">
        <a-tabs size="small">
          <a-tab-pane
            key="records"
            :tab="`${$t('record.archive.records')} (${detail?.records.length ?? 0})`"
          >
            <a-empty v-if="!detail?.records.length" />
            <div v-for="(r, i) in detail?.records" :key="i" class="arch-item">
              <div class="arch-meta">
                <a-tag :color="r.state === '已执行' ? 'green' : 'red'">{{
                  r.state
                }}</a-tag>
                <span class="arch-dim"
                  >{{ r.time }} · {{ $t('record.archive.rows') }}:
                  {{ r.affect_row }}</span
                >
              </div>
              <pre class="arch-sql">{{ r.sql }}</pre>
              <div v-if="r.error" class="arch-err">{{ r.error }}</div>
            </div>
          </a-tab-pane>
          <a-tab-pane
            key="rollbacks"
            :tab="`${$t('record.archive.rollbacks')} (${detail?.rollbacks.length ?? 0})`"
          >
            <a-empty v-if="!detail?.rollbacks.length" />
            <pre
              v-for="(r, i) in detail?.rollbacks"
              :key="i"
              class="arch-sql"
              >{{ r.sql }}</pre>
          </a-tab-pane>
          <a-tab-pane
            key="details"
            :tab="`${$t('record.archive.timeline')} (${detail?.details.length ?? 0})`"
          >
            <a-timeline style="margin-top: 8px">
              <a-timeline-item v-for="(d, i) in detail?.details" :key="i">
                <span class="arch-dim">{{ d.time }} · {{ d.username }}</span>
                <div>{{ d.action }}</div>
              </a-timeline-item>
            </a-timeline>
          </a-tab-pane>
          <a-tab-pane
            key="comments"
            :tab="`${$t('record.archive.comments')} (${detail?.comments.length ?? 0})`"
          >
            <a-empty v-if="!detail?.comments.length" />
            <div v-for="(c, i) in detail?.comments" :key="i" class="arch-item">
              <div class="arch-dim">{{ c.time }} · {{ c.username }}</div>
              <div>{{ c.content }}</div>
            </div>
          </a-tab-pane>
        </a-tabs>
      </a-spin>
    </a-drawer>
  </div>
</template>

<script lang="ts" setup>
  import { onMounted, reactive, ref } from 'vue';
  import { useI18n } from 'vue-i18n';
  import { OrderState } from '@/types';
  import {
    ArchiveDetail,
    ArchiveOrder,
    getArchiveDetail,
    getArchiveList,
  } from '@/apis/record';

  const { t } = useI18n();

  const rows = ref<ArchiveOrder[]>([]);
  const loading = ref(false);
  const page = ref(1);
  const size = 20;
  const total = ref(0);
  const searchWorkId = ref('');
  const dateRange = ref<[string, string] | null>(null);

  const pagination = reactive({
    current: 1,
    pageSize: size,
    total: 0,
    showSizeChanger: false,
    onChange: (p: number) => load(p),
  });

  const columns = [
    {
      title: t('common.order'),
      dataIndex: 'work_id',
      key: 'work_id',
      width: 200,
    },
    {
      title: t('common.table.state'),
      dataIndex: 'status',
      key: 'status',
      width: 90,
    },
    { title: 'DML/DDL', dataIndex: 'type', key: 'type', width: 80 },
    { title: t('common.table.source'), dataIndex: 'source', width: 120 },
    { title: t('common.table.schema'), dataIndex: 'data_base', width: 120 },
    { title: t('user.username'), dataIndex: 'username', width: 110 },
    { title: t('record.archive.date'), dataIndex: 'date', width: 150 },
    { title: t('common.table.remark'), dataIndex: 'text', ellipsis: true },
  ];

  const statusText = (s: number) => {
    switch (s) {
      case OrderState.REJECT:
        return t('order.state.reject');
      case OrderState.SUCCESS:
        return t('order.state.success');
      case OrderState.AUDIT:
        return t('order.state.audit');
      case OrderState.PROCESS:
        return t('order.state.process');
      case OrderState.ERROR:
        return t('order.state.error');
      case OrderState.WAIT:
        return t('order.state.wait');
      case OrderState.Undo:
        return t('record.archive.stateUndo');
      default:
        return String(s);
    }
  };

  const statusColor = (s: number) => {
    switch (s) {
      case OrderState.SUCCESS:
        return 'green';
      case OrderState.ERROR:
      case OrderState.REJECT:
        return 'red';
      case OrderState.PROCESS:
        return 'blue';
      case OrderState.WAIT:
        return 'orange';
      default:
        return 'default';
    }
  };

  const load = async (p: number) => {
    loading.value = true;
    page.value = p;
    try {
      const { data } = await getArchiveList({
        page: p,
        size,
        work_id: searchWorkId.value || undefined,
        date_from: dateRange.value?.[0],
        date_to: dateRange.value?.[1],
      });
      rows.value = data.payload.payload;
      total.value = data.payload.total;
      pagination.current = p;
      pagination.total = data.payload.total;
    } finally {
      loading.value = false;
    }
  };

  const resetFilters = () => {
    searchWorkId.value = '';
    dateRange.value = null;
    load(1);
  };

  // ---- 详情抽屉 ----
  const drawerOpen = ref(false);
  const detailLoading = ref(false);
  const detailWorkId = ref('');
  const detail = ref<ArchiveDetail | null>(null);

  const openDetail = async (wid: string) => {
    detailWorkId.value = wid;
    drawerOpen.value = true;
    detailLoading.value = true;
    detail.value = null;
    try {
      const { data } = await getArchiveDetail(wid);
      detail.value = data.payload;
    } finally {
      detailLoading.value = false;
    }
  };

  const customRow = (record: ArchiveOrder) => ({
    style: { cursor: 'pointer' },
    onClick: () => openDetail(record.work_id),
  });

  onMounted(() => load(1));
</script>

<style scoped>
  .arch-item {
    padding: 8px;
    margin-bottom: 8px;
    border: 1px solid #f0f0f0;
    border-radius: 4px;
  }
  .arch-meta {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 4px;
  }
  .arch-dim {
    opacity: 0.65;
    font-size: 12px;
  }
  .arch-sql {
    background: #fafafa;
    padding: 8px;
    border-radius: 4px;
    white-space: pre-wrap;
    word-break: break-all;
    font-size: 12px;
    margin: 0;
  }
  .arch-err {
    color: #cf1322;
    font-size: 12px;
    margin-top: 4px;
  }
</style>
