<template>
  <a-modal
    v-model:open="is_open"
    :title="$t('order.retry.title')"
    width="780px"
    :footer="null"
    @cancel="turnState"
  >
    <a-spin :spinning="loading">
      <!-- 定位失败（执行记录与工单内容不匹配）：后端会拒绝重试/继续，这里直接说明原因 -->
      <a-alert
        v-if="preview?.error"
        type="error"
        show-icon
        :message="preview.error"
        style="margin-bottom: 12px"
      ></a-alert>
      <template v-if="preview && !preview.error">
        <a-tabs size="small">
          <a-tab-pane key="failed" :tab="$t('order.retry.failed')">
            <div style="max-height: 200px; overflow: auto">
              <div
                v-for="(f, idx) in preview.failed"
                :key="idx"
                style="
                  border-bottom: 1px dashed #eee;
                  padding: 6px 0;
                  font-size: 12px;
                "
              >
                <code style="white-space: pre-wrap">{{ f.sql }}</code>
                <div style="color: #cf1322">{{ f.error || f.state }}</div>
              </div>
              <a-empty
                v-if="!preview.failed.length"
                :description="$t('order.retry.nofailed')"
              />
            </div>
          </a-tab-pane>
          <a-tab-pane key="remaining" :tab="$t('order.retry.remaining')">
            <pre
              style="
                max-height: 200px;
                overflow: auto;
                background: #f6f6f6;
                padding: 8px;
                font-size: 12px;
              "
              >{{ preview.remaining }}</pre>
          </a-tab-pane>
          <a-tab-pane
            v-if="preview.skip_available"
            key="remaining_skip"
            :tab="$t('order.retry.remaining_skip')"
          >
            <pre
              style="
                max-height: 200px;
                overflow: auto;
                background: #f6f6f6;
                padding: 8px;
                font-size: 12px;
              "
              >{{ preview.remaining_skip }}</pre>
          </a-tab-pane>
        </a-tabs>
        <!-- 两条人工介入路径：重试=把失败语句再跑一遍；跳过并继续=失败语句已在库上
             处理完（或确认放弃），从其后的语句继续执行 -->
        <a-space style="margin-top: 12px">
          <a-button
            type="primary"
            :loading="confirmLoading"
            @click="postRetry"
            >{{ $t('order.retry') }}</a-button
          >
          <a-popconfirm
            :title="$t('order.retry.skip.tips')"
            placement="top"
            @confirm="postSkip"
          >
            <a-button
              type="primary"
              ghost
              :loading="skipLoading"
              :disabled="!preview.skip_available"
              >{{ $t('order.retry.skip') }}</a-button
            >
          </a-popconfirm>
        </a-space>
      </template>
    </a-spin>
  </a-modal>
</template>

<script lang="ts" setup>
  import {
    retryOrder,
    retryPreview,
    skipAndContinue,
    RetryPreview,
  } from '@/apis/orderPostApis';
  import CommonMixins from '@/mixins/common';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { ref, Ref } from 'vue';

  const emit = defineEmits(['close']);

  const props = defineProps<{
    workId: string;
  }>();

  const preview = ref<RetryPreview | null>(null);
  const loading = ref(false);
  const confirmLoading = ref(false);
  const skipLoading = ref(false);

  // 打开即拉预览：先看清「错在哪、重试/继续分别会执行什么」再动手
  const load = async () => {
    loading.value = true;
    try {
      const { data } = await retryPreview(props.workId);
      if (data.code === 1200) {
        preview.value = data.payload;
      } else {
        message.error(data.text);
      }
    } finally {
      loading.value = false;
    }
  };

  const post = async (run: () => Promise<any>, spinning: Ref) => {
    spinning.value = true;
    try {
      const { data } = await run();
      if (data.code === 1200) {
        message.success(data.text);
        turnState();
        emit('close');
      } else {
        message.error(data.text);
      }
    } finally {
      spinning.value = false;
    }
  };

  const postRetry = () => post(() => retryOrder(props.workId), confirmLoading);
  const postSkip = () => post(() => skipAndContinue(props.workId), skipLoading);

  const { is_open, turnState } = CommonMixins();

  // CommonMixins 的 turnState 只切换可见性；打开时额外触发加载
  const open = () => {
    preview.value = null;
    turnState();
    load();
  };

  defineExpose({
    turnState: open,
  });
</script>
