<template>
  <a-row>
    <a-col :span="12">
      <a-divider orientation="left">{{ $t('setting.message.push') }}</a-divider>
      <a-form v-bind="layout">
        <a-form-item :label="$t('setting.message.hook.addr')">
          <a-input v-model:value="config.message.web_hook"></a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.message.hook.key')">
          <a-input-password
            v-model:value="config.message.key"
          ></a-input-password>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp')">
          <a-input v-model:value="config.message.host"></a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp.enabled')">
          <a-checkbox v-model:checked="config.message.ssl"></a-checkbox>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp.port')">
          <a-input-number v-model:value="config.message.port"></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp.user')">
          <a-input v-model:value="config.message.user"></a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp.password')">
          <a-input-password
            v-model:value="config.message.password"
          ></a-input-password>
        </a-form-item>
        <a-form-item :label="$t('setting.message.smtp.test')">
          <a-input v-model:value="config.message.to_user"></a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.message.mail.switch')">
          <a-switch v-model:checked="config.message.mail"></a-switch>
        </a-form-item>
        <a-form-item :label="$t('setting.message.hook.switch')">
          <a-switch v-model:checked="config.message.ding"></a-switch>
        </a-form-item>
        <a-form-item :label="$t('common.action')">
          <a-space>
            <a-button type="primary" @click="testMessageHook('ding', config)">{{
              $t('setting.message.action.hook')
            }}</a-button>
            <a-button ghost @click="testMessageHook('mail', config)">{{
              $t('setting.message.action.mail')
            }}</a-button>
          </a-space>
        </a-form-item>
      </a-form>
      <Btn :config="config" />
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
  import Btn from './btn.vue';
  import CommonMixins from '@/mixins/common';
  import { testMessageHook, Settings } from '@/apis/setting';
  import { inject } from 'vue';
  const { layout } = CommonMixins();
  const config = inject('config') as Settings;
</script>

<style scoped></style>
