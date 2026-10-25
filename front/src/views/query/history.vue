<template>
  <a-list
    item-layout="horizontal"
    :data-source="data"
    size="small"
    :pagination="{
      size: 'small',
      total: data.length,
      showTotal: (total:number) => $t('common.count', { count: total }),
    }"
  >
    <template #renderItem="{ item }">
      <a-list-item>
        <a-list-item-meta>
          <template #title>
            <a-row>
              <div style="cursor: pointer" @click="copyHistory(item)">
                <a-tooltip placement="top">
                  <template #title>
                    <span>{{ item }}</span>
                  </template>
                  {{ item.length > 30 ? `${item.substr(0, 30)}...` : item }}
                </a-tooltip>
              </div>
            </a-row>
          </template>
        </a-list-item-meta>
      </a-list-item>
    </template>
  </a-list>
</template>

<script lang="ts" setup>
  import { useOrderStore } from '@/store';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useClipboard } from '@vueuse/core';
  import { computed } from 'vue';
  import { useI18n } from 'vue-i18n';

  // legacy: 内网 http 部署没有 navigator.clipboard(安全上下文限制),
  // 退回 execCommand,与原 ClipboardJS 行为一致
  const { copy } = useClipboard({ legacy: true });

  const { t } = useI18n();

    const orderStore = useOrderStore();

  const data = computed(() => orderStore.history);

  const copyHistory = async (item: string) => {
    await copy(item);
    message.info(t('query.clip.is.paste'));
  };
</script>
