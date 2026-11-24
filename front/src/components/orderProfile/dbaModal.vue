<template>
  <a-modal
    v-model:open="is_open"
    :title="$t('order.dba.title')"
    :confirm-loading="confirmLoading"
    @ok="postDBAApproval"
  >
    <a-alert
      class="dba-alert"
      type="warning"
      show-icon
      :message="$t('order.dba.tips')"
    />
    <a-form layout="vertical">
      <a-form-item :label="$t('common.action')" required>
        <a-radio-group v-model:value="action">
          <a-radio value="approve">{{ $t('order.dba.approve') }}</a-radio>
          <a-radio value="reject">{{ $t('order.dba.reject') }}</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="$t('order.dba.reason')" required>
        <a-textarea
          v-model:value="reason"
          :rows="4"
          :placeholder="$t('order.dba.reason.required')"
        />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script lang="ts" setup>
  import { reviewDBAApproval } from '@/apis/orderPostApis';
  import CommonMixins from '@/mixins/common';
  import { App } from 'ant-design-vue';
  import { ref } from 'vue';
  import { useI18n } from 'vue-i18n';

  const emit = defineEmits(['close']);

  const props = defineProps<{
    workId: string;
  }>();

  const { message } = App.useApp();
  const { t } = useI18n();

  const action = ref<'approve' | 'reject'>('approve');
  const reason = ref('');
  const confirmLoading = ref(false);

  // 高危 MongoDB 命令不能匿名通过：必须留下 DBA、动作和理由。
  const postDBAApproval = async () => {
    if (!reason.value.trim()) {
      message.warning(t('order.dba.reason.required'));
      return;
    }
    confirmLoading.value = true;
    const { data } = await reviewDBAApproval({
      work_id: props.workId,
      action: action.value,
      reason: reason.value.trim(),
    });
    confirmLoading.value = false;
    if (data.code !== 1200) return;
    message.success(data.text);
    reason.value = '';
    action.value = 'approve';
    turnState();
    emit('close');
  };

  const { is_open, turnState } = CommonMixins();

  defineExpose({
    turnState,
  });
</script>

<style scoped>
  .dba-alert {
    margin-bottom: 16px;
  }
</style>
