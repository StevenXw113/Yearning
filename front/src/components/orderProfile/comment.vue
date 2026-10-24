<template>
  <a-list
    id="comment"
    style="height: 300px; overflow: auto"
    :header="$t('common.count', { count: data.length })"
    item-layout="horizontal"
    :data-source="data"
  >
    <template #renderItem="{ item }">
      <a-list-item>
        <a-comment :author="item.username" :avatar="icon">
          <template #content>
            <p>{{ item.content }}</p>
          </template>
          <template #datetime>
            <a-tooltip :title="item.time">
              <span>{{ dayjs(item.time).fromNow() }}</span>
            </a-tooltip>
          </template>
        </a-comment>
      </a-list-item>
    </template>
  </a-list>
  <a-comment>
    <a-avatar :src="comment" />
    <slot name="content"></slot>
  </a-comment>
  <a-form>
    <a-form-item>
      <a-textarea v-model:value="content" :rows="4" />
    </a-form-item>
    <a-form-item>
      <a-button html-type="submit" type="primary" @click="postComment">{{
        $t('order.profile.comment.add')
      }}</a-button>
    </a-form-item>
  </a-form>
</template>

<script lang="ts" setup>
import { wsHeartbeat } from '@/socket';
  import dayjs, { extend } from 'dayjs';
  import relativeTime from 'dayjs/plugin/relativeTime';
  import customParseFormat from 'dayjs/plugin/customParseFormat';
  import { Comment, postOrderComment } from '@/apis/orderPostApis';
  import { onMounted, ref, nextTick, onUpdated } from 'vue';
  import { useUserStore } from '@/store';
  import comment from '@/assets/comment/rockets.svg';
  import icon from '@/assets/comment/comment.svg';
  import { useWebSocket } from '@vueuse/core';
  import { checkSchema } from '@/lib';
  import { COMMON_URI } from '@/config/request';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';

  extend(relativeTime);
  extend(customParseFormat);

  const props = defineProps<{
    workId: string;
  }>();

  const { t } = useI18n();

  const data = ref<Comment[]>([]);

  const content = ref('');

  const scrollTop = () => {
    nextTick(() => {
      let h = document.getElementById('comment') as HTMLElement;
      h.scrollTop = h.scrollHeight;
    });
  };

    const userStore = useUserStore();

  const postComment = async () => {
    await postOrderComment({
      work_id: props.workId,
      content: content.value,
    });
    data.value.push({
      username: userStore.account.user,
      time: dayjs().format('YY-MM-DD HH:mm'),
      content: content.value,
      work_id: props.workId,
    } as Comment);
    content.value = '';
    scrollTop();
  };

  onUpdated(() => {
    scrollTop();
  });

  onMounted(async () => {
    useWebSocket(
      `${checkSchema()}${COMMON_URI}/fetch/comment?work_id=${props.workId}`,
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
          data.value = JSON.parse(event.data);
        },
      }
    );
  });
</script>
