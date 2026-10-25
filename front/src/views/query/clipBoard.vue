<template>
  <a-card
    :title="clips.title"
    hoverable
    :body-style="{ height: '200px' }"
    size="small"
  >
    <template #extra>
      <a-space :size="15">
        <CopyOutlined @click="copyClip"></CopyOutlined>
        <CloseOutlined @click="delBtn" />
      </a-space>
    </template>
    <div>{{ clips.desc }}</div>
  </a-card>
</template>

<script lang="ts" setup>
  import { Clip } from '@/types';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useClipboard } from '@vueuse/core';
  import { useI18n } from 'vue-i18n';
  import { CloseOutlined, CopyOutlined } from '@ant-design/icons-vue';

  const props = defineProps<{
    idRef: number;
    clips: Clip;
  }>();

  const emit = defineEmits(['delBtn']);

  const { t } = useI18n();

  // legacy: 内网 http 部署没有 navigator.clipboard(安全上下文限制),
  // 退回 execCommand,与原 ClipboardJS 行为一致
  const { copy } = useClipboard({ legacy: true });

  const copyClip = async () => {
    await copy(props.clips.desc);
    message.info(t('query.clip.is.paste'));
  };

  const delBtn = () => {
    emit('delBtn', props.idRef);
  };
</script>
