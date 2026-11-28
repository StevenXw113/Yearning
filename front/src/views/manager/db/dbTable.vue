<template>
  <a-card>
    <DBTableSearch @search="search"></DBTableSearch>
    <c-table ref="tbl" :tbl-ref="tblRef" :scroll="{ x: 960 }">
      <template #bodyCell="{ column, text, record }">
        <template v-if="column.dataIndex === 'action'">
          <a-space size="small" wrap>
            <a-button
              type="primary"
              size="small"
              ghost
              @click="fillInfo(record)"
            >
              {{ $t('common.profile') }}</a-button
            >
            <a-popconfirm
              :title="$t('db.delete.tips')"
              @confirm="deleteSource(record.source_id).then(() => tbl.manual())"
            >
              <a-button type="primary" size="small" danger ghost
                >{{ $t('common.delete') }}
              </a-button>
            </a-popconfirm>
          </a-space>
        </template>
        <template v-if="column.dataIndex === 'db_type'">
          <a-tag>{{ dbTypeLabel(text) }}</a-tag>
        </template>
        <template v-if="column.dataIndex === 'environment_tag'">
          <a-tag v-if="record.environment_tag" color="red">
            {{ record.environment_tag }}
          </a-tag>
          <a-tag v-else-if="record.is_production" color="orange">
            production
          </a-tag>
          <span v-else>—</span>
        </template>
        <template v-if="column.dataIndex === 'is_query'">
          <a-tag v-if="text === 2" color="green">{{ $t('db.rw') }}</a-tag>
          <a-tag v-if="text === 1" color="blue">{{ $t('db.r') }}</a-tag>
          <a-tag v-if="text === 0" color="orange">{{ $t('db.w') }}</a-tag>
        </template>
      </template>
    </c-table>
  </a-card>

  <DBModal ref="modal" :idc="idc" :rules="rules" @edited="refresh"></DBModal>
</template>

<script lang="ts" setup>
  import {
    DBExpr,
    Source,
    DBParams,
    getSourceList,
    deleteSource,
    dbTypeLabel,
  } from '@/apis/db';
  import DBModal from './dbModal.vue';
  import DBTableSearch from './dbTableSearch.vue';
  import { ref, reactive, onMounted } from 'vue';
  import { tableRef } from '@/components/table';
  import { useI18n } from 'vue-i18n';
  import { CustomRule, getCustomRulesList } from '@/apis/rules';

  const { t } = useI18n();

  const rules = ref([] as CustomRule[]);

  const tblRef = reactive<tableRef>({
    col: [
      { title: t('common.table.name'), dataIndex: 'source' },
      { title: t('common.table.db_type'), dataIndex: 'db_type', width: 120 },
      { title: t('common.table.env'), dataIndex: 'idc', width: 112 },
      {
        title: t('db.environment.tag'),
        dataIndex: 'environment_tag',
        width: 130,
      },
      { title: 'IP', dataIndex: 'ip' },
      {
        title: t('common.table.type'),
        dataIndex: 'is_query',
        width: 112,
        className: 'table-status-cell',
      },
      {
        title: t('common.action'),
        dataIndex: 'action',
        width: 152,
        className: 'table-action-cell',
        fixed: 'right',
      },
    ],
    data: [] as Source[],
    pageCount: 1,
    expr: {
      source: '',
      ip: '',
      idc: '',
      is_query: -1,
    } as DBExpr,
    fn: async (expr: DBParams) => {
      const { data } = await getSourceList(expr);
      tblRef.data = data.payload.data;
      tblRef.pageCount = data.payload.page;
    },
  });

  const modal = ref();

  const tbl = ref();

  const idc = ref([] as string[]);

  const search = (vl: DBExpr) => {
    tblRef.expr = vl;
    tbl.value.manual(true);
  };

  const fillInfo = (vl: Source) => {
    modal.value.fillInfo(vl);
  };

  // 刷新列表:编辑弹窗保存后本组件自行调用;新建表单提交后由父组件(db.vue)中继调用
  const refresh = () => {
    tbl.value?.manual();
  };

  defineExpose({
    refresh,
  });

  onMounted(async () => {
    const { data } = await getCustomRulesList();
    rules.value = data.payload;
  });
</script>
