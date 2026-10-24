<template>
  <a-modal
    v-model:visible="is_open"
    :title="$t('order.mark.title')"
    :confirm-loading="confirmLoading"
    @ok="postMark"
  >
    <p style="margin-bottom: 8px">{{ $t('order.mark.tips') }}</p>
    <a-textarea
      v-model:value="content"
      :rows="5"
      :placeholder="$t('order.mark.placeholder')"
    ></a-textarea>
  </a-modal>
</template>

<script lang="ts" setup>
  import { markOrderFinished } from '@/apis/orderPostApis';
  import CommonMixins from '@/mixins/common';
  import { message } from 'ant-design-vue';
  import { ref } from 'vue';
  import { useI18n } from 'vue-i18n';

  const emit = defineEmits(['close']);

  const props = defineProps<{
    workId: string;
  }>();

  const { t } = useI18n();

  const content = ref('');

  const confirmLoading = ref(false);

  // 手动标记完成必须填写处理说明，便于审计追溯（后端同样强校验）
  const postMark = async () => {
    if (!content.value.trim()) {
      message.warning(t('order.mark.placeholder'));
      return;
    }
    confirmLoading.value = true;
    const { data } = await markOrderFinished(props.workId, content.value);
    confirmLoading.value = false;
    if (data.code === 1200) {
      message.success(data.text);
      turnState();
      emit('close');
    }
  };

  const { is_open, turnState } = CommonMixins();

  defineExpose({
    turnState,
  });
</script>
