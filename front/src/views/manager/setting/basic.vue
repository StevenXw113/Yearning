<template>
  <a-row>
    <a-col :span="10">
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
      <a-alert message="Warning" type="warning" show-icon>
        <template #icon>
          <SmileOutlined></SmileOutlined>
        </template>
        <template #description>
          <div v-html="$t('setting.ldap.alert')"></div>
        </template>
      </a-alert>
    </a-col>
    <a-col :span="11" offset="1">
      <a-divider orientation="left">{{ $t('setting.ldap') }}</a-divider>
      <a-form v-bind="layout">
        <a-form-item :label="$t('setting.ldap.url')">
          <a-input
            v-model:value="config.ldap.url"
            :placeholder="$t('setting.ldap.url.tips')"
          >
          </a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.ssl')">
          <a-checkbox v-model:checked="config.ldap.ldaps"></a-checkbox>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.dn')">
          <a-input
            v-model:value="config.ldap.user"
            :placeholder="$t('setting.ldap.dn.tips')"
          >
          </a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.password')">
          <a-input-password
            v-model:value="config.ldap.password"
            :placeholder="$t('setting.ldap.password.tips')"
          ></a-input-password>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.filter')">
          <a-input
            v-model:value="config.ldap.type"
            :placeholder="$t('setting.ldap.filter.tips')"
          >
          </a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.sc')">
          <a-input
            v-model:value="config.ldap.sc"
            placeholder="LDAP Search Base"
          ></a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.map')">
          <a-textarea
            v-model:value="config.ldap.map"
            :rows="5"
            allow-clear
            :placeholder="$t('setting.ldap.map.tips')"
          />
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.test.user')">
          <a-input
            v-model:value="config.ldap.test_user"
            placeholder="LDAP Test User"
          >
          </a-input>
        </a-form-item>
        <a-form-item :label="$t('setting.ldap.test.password')">
          <a-input-password
            v-model:value="config.ldap.test_password"
            placeholder="LDAP Test User Password"
          >
            >
          </a-input-password>
        </a-form-item>
        <a-form-item :label="$t('common.action')">
          <a-button type="primary" @click="testMessageHook('ldap', config)">{{
            $t('setting.ldap.test')
          }}</a-button>
        </a-form-item>
        <Btn :config="config" />
      </a-form>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
  import Btn from './btn.vue';
  import { SmileOutlined } from '@ant-design/icons-vue';
  import CommonMixins from '@/mixins/common';
  import { testMessageHook, Settings } from '@/apis/setting';
  import { inject } from 'vue';
  const { layout } = CommonMixins();
  const config = inject('config') as Settings;
</script>

<style scoped></style>
