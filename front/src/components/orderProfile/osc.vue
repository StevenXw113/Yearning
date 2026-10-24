<template>
  <pre style="height: 500px; overflow: auto">
            {{ pre }}
      </pre
  >
</template>

<script lang="ts" setup>
import { wsHeartbeat } from '@/socket';
  import { onMounted, ref } from 'vue';
  import { useUserStore } from '@/store';
  import { checkSchema } from '@/lib';
  import { COMMON_URI } from '@/config/request';
  import { useWebSocket } from '@vueuse/core';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';

  const pre = ref('');

    const userStore = useUserStore();

  const props = defineProps<{
    workId: string;
  }>();

  const { t } = useI18n();

  onMounted(() => {
    useWebSocket(
      `${checkSchema()}${COMMON_URI}/audit/order/osc?work_id=${props.workId}`,
      {
        autoReconnect: {
          retries: 3,
          delay: 1000,
          onFailed() {
            message.error(t('query.ws.error'));
          },
        },
        heartbeat: wsHeartbeat(),
        protocols: [userStore.account.token],
        onMessage: (e, event) => {
          pre.value = event.data;
        },
      }
    );
  });
</script>

<style></style>
