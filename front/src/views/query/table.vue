<template>
  <a-space>
    <span>{{ $t('order.query.execute.time') }}:{{ executeTime }} ms</span>
  </a-space>
  <a-tabs v-model:activeKey="activeKey">
    <a-tab-pane
      v-for="(i, idx) in results"
      :key="idx"
      :tab="`${$t('common.result')} ${idx + 1}`"
    >
      <template v-if="isExport">
        <a-button
          size="small"
          type="primary"
          ghost
          @click="downloadXLS(i.data, i.field)"
          >{{ $t('common.export') }}
        </a-button>
        <br />
        <br />
      </template>
      <div>
        <a-table
          size="small"
          bordered
          :scroll="{ x: i.field.length * 200 + 100 }"
          table-layout="fixed"
          :columns="i.field"
          :data-source="i.data"
          :pagination="{
            showTotal: (total: number) => $t('common.count', { count: total }),
          }"
          @resize-column="handleResizeColumn"
        >
          <template #bodyCell="{ text }">
            <a-tooltip placement="topLeft">
              <template #title>
                <span>{{ text }}</span>
              </template>
              <div class="ellipsis" @click="copyText(text)">{{ text }}</div>
            </a-tooltip>
          </template>
        </a-table>
      </div>
    </a-tab-pane>
  </a-tabs>
</template>

<script lang="ts" setup>
  import type * as XLSXType from 'xlsx';
  import { computed, ref, watch } from 'vue';
  import { useCommonStore } from '@/store';
  import router from '@/router';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';
  import { encode, decode } from '@msgpack/msgpack';
  import { useClipboard } from '@vueuse/core';

  defineProps<{
    height: number;
    id: string;
  }>();

  const activeKey = ref(0);

  // legacy: 内网 http 部署没有 navigator.clipboard(安全上下文限制),
  // 退回 execCommand,与原 vue-clipboard3 行为一致
  const { copy } = useClipboard({ legacy: true });

  const commonStore = useCommonStore();

  const executeTime = ref(0);

  const { t } = useI18n();

  const isExport = ref(false);

  let results = ref<any[]>();

  const handleResizeColumn = (w: number, col: { width: number }) => {
    col.width = w;
  };

  // xlsx 动态 import:只有点导出才加载,查询路由不必常驻这份代码
  const downloadXLS = async (data: any, title: any) => {
    const XLSX = await import('xlsx');
    const options = {
      '!cols': [
        { wpx: 100 },
        { wpx: 100 },
        { wpx: 100 },
        { wpx: 100 },
        { wpx: 100 },
      ],
    };

    const worksheet: XLSXType.WorkSheet = XLSX.utils.json_to_sheet(data, {
      header: title.map((item: any) => item.title),
    });
    worksheet['!cols'] = options['!cols'];
    const workbook: XLSXType.WorkBook = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(workbook, worksheet, 'Sheet1');
    XLSX.writeFile(workbook, 'Yearning_Table_Data.xlsx');
  };

  const sock = computed(() => commonStore.sock);

  watch(
    () => sock.value.data as any,
    (val: Blob) => {
      val !== null ? recv(val) : null;
    },
  );

  const runResults = (schema: string, sql: string) => {
    commonStore.SET_SPINNING();
    const encoded: Uint8Array = encode({ type: 4, sql: sql, schema: schema });
    sock.value.send(encoded);
    const ws = sock.value.ws as any;
    if (ws.readyState === 1) {
      ws.onmessage = recv;
      return;
    }
    message.error(t('query.ws.error'));
    commonStore.SET_DISABLED_SPINNING();
  };

  const recv = async (e: any) => {
    const h = e.data as Blob;
    if (h === undefined) {
      return;
    }
    if (h.size > 0) {
      const resp = decode(await h.arrayBuffer()) as any;
      if (resp.heartbeat === 'pong') {
        return;
      }
      resp.status ? (router.go(-1), message.error(t('query.expire'))) : null;
      if (resp.error !== '') {
        message.error(resp.error);
        results.value = [];
      } else {
        isExport.value = resp.export;
        resp.results !== null
          ? (results.value = resp.results)
          : (results.value = []);
        executeTime.value = resp.query_time;
        activeKey.value = 0;
        // 查询侧规则命中（如未锚定的 $regex）：只提示，结果照常展示
        resp.audit ? message.warning(resp.audit) : null;
      }
    }
    commonStore.SET_DISABLED_SPINNING();
  };

  const copyText = async (text: string) => {
    try {
      await copy(text);
      message.info(t('query.clip.is.paste'));
    } catch (e) {
      console.error(e);
    }
  };

  defineExpose({
    runResults,
  });
</script>

<style lang="less" scoped>
  .ellipsis {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: grab;
  }
</style>
