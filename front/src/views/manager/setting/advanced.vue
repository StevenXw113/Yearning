<template>
  <a-row>
    <a-col :span="10">
      <a-divider orientation="left">{{ $t('setting.adv') }}</a-divider>
      <a-form v-bind="layout">
        <a-form-item :label="$t('setting.adv.query.limit')">
          <a-input-number
            v-model:value="config.other.limit"
            :min="1"
          ></a-input-number>
        </a-form-item>
        <a-form-item :label="$t('setting.adv.env')">
          <template v-for="item in config.other.idc" :key="item">
            <a-tag color="#B38D57" closable @close="closeTag(item)">{{
              item
            }}</a-tag>
          </template>

          <br />
          <br />
          <a-space>
            <a-input
              v-model:value="config.other.force"
              :placeholder="$t('setting.adv.env.tips')"
            ></a-input>
            <a-button @click="pushEnv">{{
              $t('setting.adv.env.add')
            }}</a-button>
          </a-space>
        </a-form-item>
        <a-form-item :label="$t('setting.adv.query.open')">
          <a-switch v-model:checked="config.other.query"></a-switch>
        </a-form-item>
        <a-form-item :label="$t('setting.adv.query.register')">
          <a-switch v-model:checked="config.other.register"></a-switch>
        </a-form-item>
        <a-form-item :label="$t('setting.adv.query.export')">
          <a-radio-group v-model:value="config.other.export" name="radioGroup">
            <a-radio :value="false">{{ $t('common.no') }}</a-radio>
            <a-radio :value="true">{{ $t('common.yes') }}</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item :label="$t('setting.adv.query.expire')">
          <a-input-number v-model:value="config.other.ex_query_time" :min="1">
          </a-input-number>
          {{ $t('setting.adv.query.mins') }}
        </a-form-item>
        <a-form-item :label="$t('setting.adv.domain')">
          <a-input v-model:value="config.other.domain" />
        </a-form-item>
      </a-form>
    </a-col>
    <a-col :span="11" offset="1">
      <a-divider orientation="left">{{ $t('setting.data.clear') }}</a-divider>
      <a-form v-bind="layout">
        <a-form-item :label="$t('setting.data.clear.order')">
          <a-space>
            <a-range-picker
              v-model:value="config.other.overdue"
              show-time
              format="YYYY/MM/DD HH:mm"
              :presets="dateRanges"
            />
            <a-popconfirm
              :title="$t('setting.data.clear.tips')"
              @confirm="clearOrder"
            >
              <a-button>{{ $t('common.delete') }}</a-button>
            </a-popconfirm>
          </a-space>
        </a-form-item>
        <a-form-item :label="$t('setting.data.clear.query')">
          <a-space>
            <a-range-picker
              v-model:value="config.other.query_expire"
              show-time
              format="YYYY/MM/DD HH:mm"
              :presets="dateRanges"
            />
            <a-popconfirm
              :title="$t('setting.data.clear.tips')"
              @confirm="clearQuery"
            >
              <a-button>{{ $t('common.delete') }}</a-button>
            </a-popconfirm>
          </a-space>
        </a-form-item>
        <a-alert message="Warning" type="warning" show-icon>
          <template #icon>
            <SmileOutlined></SmileOutlined>
          </template>
          <template #description>
            <div v-html="$t('setting.data.clear.alert')"></div>
          </template>
        </a-alert>
      </a-form>
      <br />
      <Btn :config="config" />
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
  import CommonMixins from '@/mixins/common';
  import Btn from './btn.vue';
  import { Settings, deleteOrderRecords } from '@/apis/setting';
  import { inject, ref } from 'vue';
  import dayjs from 'dayjs';
  import { SmileOutlined } from '@ant-design/icons-vue';
  const { layout } = CommonMixins();

  const config = ref(inject('config') as Settings);

  const clearQuery = async () => {
    if (config.value.other.query_expire !== undefined) {
      await deleteOrderRecords({
        date: config.value.other.query_expire.map((item) =>
          item.format('YYYY-MM-DD HH:mm')
        ),
        tp: true,
      });
    }
  };

  const dateRanges =
    sessionStorage.getItem('lang') === 'en_US'
      ? [
          {
            label: 'this month',
            value: [dayjs().startOf('month'), dayjs().endOf('month')],
          },
        ]
      : [
          {
            label: '本月',
            value: [dayjs().startOf('month'), dayjs().endOf('month')],
          },
        ];

  const pushEnv = () => {
    config.value.other.idc.push(config.value.other.force);
    config.value.other.force = '';
  };

  const clearOrder = async () => {
    if (config.value.other.overdue !== undefined) {
      await deleteOrderRecords({
        date: config.value.other.overdue.map((item) =>
          item.format('YYYY-MM-DD HH:mm')
        ),
        tp: false,
      });
    }
  };

  const closeTag = (removedTag: string) => {
    const tags = config.value.other.idc.filter((tag) => tag !== removedTag);
    config.value.other.idc = tags;
  };
</script>

<style scoped></style>
