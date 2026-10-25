<template>
  <a-card :title="$t('user.mfa.title')" style="margin-top: 16px">
    <template #extra>
      <a-tag :color="enabled ? 'green' : 'default'">
        {{ enabled ? $t('user.mfa.on') : $t('user.mfa.off') }}
      </a-tag>
    </template>

    <!-- 已绑定：解绑入口（需当前动态码，防止拿到会话的人直接关掉二级认证） -->
    <template v-if="enabled">
      <a-typography-paragraph type="secondary">
        {{ $t('user.mfa.bound.tips') }}
      </a-typography-paragraph>
      <a-space>
        <a-input-password
          v-model:value="disableCode"
          :placeholder="$t('user.mfa.code')"
          style="width: 160px"
          :maxlength="6"
        />
        <a-popconfirm :title="$t('user.mfa.disable.tips')" @confirm="doDisable">
          <a-button danger ghost>{{ $t('user.mfa.disable') }}</a-button>
        </a-popconfirm>
      </a-space>
    </template>

    <!-- 未绑定：扫码/录密钥 → 输入动态码确认 -->
    <template v-else-if="setup">
      <a-row :gutter="24">
        <a-col :xs="24" :md="10" style="text-align: center">
          <canvas ref="qrRef" width="200" height="200"></canvas>
          <div style="word-break: break-all; color: #888; font-size: 12px">
            {{ $t('user.mfa.manual') }}:
            <a-typography-paragraph
              :copyable="{ text: setup.secret }"
              style="display: inline"
              code
            >
              {{ setup.secret }}
            </a-typography-paragraph>
          </div>
        </a-col>
        <a-col :xs="24" :md="14">
          <a-typography-paragraph>
            {{ $t('user.mfa.setup.tips') }}
          </a-typography-paragraph>
          <a-space direction="vertical">
            <a-space>
              <a-input
                v-model:value="enableCode"
                :placeholder="$t('user.mfa.code')"
                style="width: 160px; letter-spacing: 4px"
                :maxlength="6"
                @press-enter="doEnable"
              />
              <a-button type="primary" :loading="confirming" @click="doEnable">
                {{ $t('user.mfa.enable') }}
              </a-button>
            </a-space>
            <a-button size="small" @click="regenSetup">
              {{ $t('user.mfa.regen') }}
            </a-button>
          </a-space>
        </a-col>
      </a-row>
    </template>

    <!-- 初始：开始绑定 -->
    <template v-else>
      <a-typography-paragraph type="secondary">
        {{ $t('user.mfa.intro') }}
      </a-typography-paragraph>
      <a-button type="primary" ghost @click="regenSetup">
        {{ $t('user.mfa.start') }}
      </a-button>
    </template>
  </a-card>
</template>

<script lang="ts" setup>
  import { nextTick, onMounted, ref } from 'vue';
  import QRCode from 'qrcode';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import {
    mfaSetup,
    mfaStatus,
    mfaEnable,
    mfaDisable,
    MfaSetupResp,
  } from '@/apis/user';

  const enabled = ref(false);
  const setup = ref<MfaSetupResp | null>(null);
  const enableCode = ref('');
  const disableCode = ref('');
  const confirming = ref(false);
  const qrRef = ref<HTMLCanvasElement>();

  // 拉取待绑定密钥并渲染二维码（qrcode 库本地渲染，密钥不出浏览器）
  const regenSetup = async () => {
    const { data } = await mfaSetup();
    if (data.code !== 1200) {
      message.error(data.text);
      return;
    }
    setup.value = data.payload;
    enableCode.value = '';
    await nextTick();
    if (qrRef.value && setup.value) {
      await QRCode.toCanvas(qrRef.value, setup.value.uri, { width: 200 });
    }
  };

  const doEnable = async () => {
    if (!enableCode.value.trim()) return;
    confirming.value = true;
    try {
      const { data } = await mfaEnable(enableCode.value.trim());
      if (data.code === 1200) {
        message.success(data.text);
        setup.value = null;
        enabled.value = true;
      } else {
        message.error(data.text);
      }
    } finally {
      confirming.value = false;
    }
  };

  const doDisable = async () => {
    if (!disableCode.value.trim()) {
      message.warning('请输入当前动态码');
      return;
    }
    const { data } = await mfaDisable(disableCode.value.trim());
    if (data.code === 1200) {
      message.success(data.text);
      enabled.value = false;
      disableCode.value = '';
    } else {
      message.error(data.text);
    }
  };

  onMounted(async () => {
    const { data } = await mfaStatus();
    if (data.code === 1200) {
      enabled.value = !!data.payload.enabled;
    }
  });
</script>
