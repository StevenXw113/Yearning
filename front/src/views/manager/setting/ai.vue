<template>
  <a-row>
    <a-col :span="10">
      <a-form>
        <a-form-item :label="$t('setting.ai.base_url')">
          <a-input
            v-model:value="config.ai.base_url"
            :placeholder="$t('setting.ai.base_url.tips')"
          />
        </a-form-item>
        <a-form-item :label="$t('setting.ai.proxy_url')">
          <a-input
            v-model:value="config.ai.proxy_url"
            :placeholder="$t('setting.ai.proxy_url.tips')"
          />
        </a-form-item>
        <a-form-item :label="$t('setting.ai.api_key')">
          <a-input-password
            v-model:value="config.ai.api_key"
            :placeholder="$t('setting.ai.api_key.tips')"
          />
        </a-form-item>
        <a-form-item :label="$t('setting.ai.model')">
          <a-input
            v-model:value="config.ai.model"
            :placeholder="$t('setting.ai.model.tips')"
          />
        </a-form-item>
        <a-form-item :label="$t('setting.ai.temperature')">
          <a-input-number
            v-model:value="config.ai.temperature"
            :min="0"
            :step="0.1"
          ></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.top')">
          <a-input-number
            v-model:value="config.ai.top_p"
            :min="0"
            :step="0.1"
          ></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.presence_penalty')">
          <a-input-number
            v-model:value="config.ai.presence_penalty"
            :step="0.1"
          ></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.frequency_penalty')">
          <a-input-number
            v-model:value="config.ai.frequency_penalty"
            :step="0.1"
          ></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.max_tokens')">
          <a-input-number
            v-model:value="config.ai.max_tokens"
            :min="1"
          ></a-input-number>
        </a-form-item>
      </a-form>
    </a-col>
    <a-divider type="vertical" />
    <a-col :span="10">
      <a-form>
        <a-form-item :label="$t('setting.ai.advisor_prompt')">
          <a-textarea
            v-model:value="config.ai.advisor_prompt"
            :rows="8"
          ></a-textarea>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.sql_gen_prompt')">
          <a-textarea
            v-model:value="config.ai.sql_gen_prompt"
            :rows="8"
          ></a-textarea>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.sql_agent_prompt')">
          <a-textarea
            v-model:value="config.ai.sql_agent_prompt"
            :rows="8"
          ></a-textarea>
        </a-form-item>
      </a-form>
      <Btn />
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
  import Btn from './btn.vue';
  import { Settings } from '@/apis/setting';
  import { inject, ref } from 'vue';
  const config = ref(inject('config') as Settings);
</script>

<style scoped></style>
