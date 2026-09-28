<template>
  <a-row>
    <a-col :span="10">
      <a-form>
        <a-form-item :label="$t('setting.ai.protocol')">
          <a-select
            v-model:value="config.ai.protocol"
            @change="onProtocolChange"
          >
            <a-select-option value="openai">{{
              $t('setting.ai.protocol.openai')
            }}</a-select-option>
            <a-select-option value="deepseek">{{
              $t('setting.ai.protocol.deepseek')
            }}</a-select-option>
            <a-select-option value="anthropic">{{
              $t('setting.ai.protocol.anthropic')
            }}</a-select-option>
            <a-select-option value="responses">{{
              $t('setting.ai.protocol.responses')
            }}</a-select-option>
          </a-select>
        </a-form-item>
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
        <a-form-item :label="$t('setting.ai.mongo_advisor_prompt')">
          <a-textarea
            v-model:value="config.ai.mongo_advisor_prompt"
            :rows="8"
            :placeholder="$t('setting.ai.mongo_prompt.tips')"
          ></a-textarea>
        </a-form-item>
        <a-form-item :label="$t('setting.ai.mongo_sql_gen_prompt')">
          <a-textarea
            v-model:value="config.ai.mongo_sql_gen_prompt"
            :rows="8"
            :placeholder="$t('setting.ai.mongo_prompt.tips')"
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
  // 历史配置里没有 protocol，下拉会显示空白；缺省按 OpenAI 兼容处理
  if (!config.value.ai.protocol) {
    config.value.ai.protocol = 'openai';
  }

  // 各协议的默认接入参数：换协议时同步改写，免得把 OpenAI 的地址/模型留在 Claude 上。
  // 地址要给全（含版本段）：OpenAI / Anthropic 在 /v1 下，DeepSeek 的兼容端点在根路径。
  // model 为空表示该协议不预设模型（保持用户当前填写）。
  const protocolPresets: Record<string, { base_url: string; model: string }> = {
    openai: { base_url: 'https://api.openai.com/v1', model: '' },
    deepseek: {
      base_url: 'https://api.deepseek.com',
      model: 'deepseek-flash',
    },
    responses: {
      base_url: 'https://api.openai.com/v1',
      model: 'gpt-5.6-terra',
    },
    anthropic: {
      base_url: 'https://api.anthropic.com/v1',
      model: 'claude-opus-5-5',
    },
  };

  const presetBaseUrls = Object.values(protocolPresets).map((p) => p.base_url);
  // 常见默认模型名：命中即视为「没自定义过」，可被新协议的预设覆盖
  const presetModels = [
    'gpt-3.5-turbo',
    'gpt-4o',
    'gpt-4o-mini',
    'gpt-5.6-terra',
    'claude-opus-5-5',
    'claude-sonnet-5',
    'claude-haiku-4-5',
    'deepseek-chat',
    'deepseek-flash',
  ];

  const onProtocolChange = (protocol: string) => {
    const preset = protocolPresets[protocol];
    if (!preset) {
      return;
    }
    const ai = config.value.ai;
    // 只在字段为空、或仍是预设值时替换；自建网关、自选模型保持不动
    if (!ai.base_url || presetBaseUrls.includes(ai.base_url.trim())) {
      ai.base_url = preset.base_url;
    }
    if (preset.model && (!ai.model || presetModels.includes(ai.model.trim()))) {
      ai.model = preset.model;
    }
  };
</script>

<style scoped>
/* 设置项标签偏长且长短不一：12px 字号 + 行距 10px，标签最小 8.5em 让各行的控件左对齐，
   更长的标签自动撑开，不会被输入框截断 */
:deep(.ant-form-item) {
  margin-bottom: 10px;
}
:deep(.ant-form-item .ant-form-item-label) {
  min-width: 8.5em;
  font-size: 12px;
}
</style>
