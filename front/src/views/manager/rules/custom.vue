<template>
  <a-card ref="card">
    <a-row>
      <a-col :span="19">
        <a-input-search
          placeholder="input search text"
          enter-button
          style="max-width: 320px"
          @search="onSearch"
        />
      </a-col>
      <a-col :span="4" offset="1">
        <a-button
          type="primary"
          @click="
            is_open = true;
            ist_add = true;
          "
          >{{ $t('common.add') }}</a-button
        >
      </a-col>
    </a-row>
    <br />
    <a-table bordered :columns="col" :data-source="list">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'ref_sources'">
          <a-tag v-for="s in record.ref_sources || []" :key="s">{{ s }}</a-tag>
          <span v-if="!record.ref_count">{{ $t('ruleRefNone') }}</span>
        </template>
        <template v-if="column.dataIndex === 'action'">
          <a-button
            size="small"
            @click="
              () => {
                r.onRules(record.audit_role, record.id);
                desc = record.desc;
                turnState();
              }
            "
            >{{ $t('common.profile') }}</a-button
          >
          <a-popconfirm
            :title="$t('ruleDeleteConfirm')"
            @confirm="onDelete(record.id)"
          >
            <a-button
              type="primary"
              size="small"
              danger
              ghost
              style="margin-left: 8px"
              >{{ $t('common.delete') }}</a-button
            >
          </a-popconfirm>
        </template>
      </template>
    </a-table>
  </a-card>

  <!-- 自定义审核规则：内置规则编译在引擎里，这里补文本层正则检查 -->
  <a-card style="margin-top: 16px">
    <a-row>
      <a-col :span="17">
        <b>{{ $t('customRule') }}</b>
        <div style="color: #8c8c8c; font-size: 12px">
          {{ $t('customRuleHelp') }}
        </div>
      </a-col>
      <a-col :span="6" offset="1" style="text-align: right">
        <a-button type="primary" @click="openRule()">{{
          $t('customRuleAdd')
        }}</a-button>
      </a-col>
    </a-row>
    <br />
    <a-table
      bordered
      size="small"
      :columns="ruleCol"
      :data-source="ruleList"
      :pagination="{ pageSize: 10 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'pattern'">
          <code>{{ record.pattern }}</code>
        </template>
        <template v-if="column.dataIndex === 'level'">
          {{
            record.level === 'warning'
              ? $t('customRuleLevelWarn')
              : $t('customRuleLevelError')
          }}
        </template>
        <template v-if="column.dataIndex === 'enabled'">
          <a-switch
            :checked="record.enabled === 1"
            @change="(v: boolean) => toggleRule(record, v)"
          />
        </template>
        <template v-if="column.dataIndex === 'action'">
          <a-button size="small" @click="openRule(record)">
            {{ $t('common.edit') }}
          </a-button>
          <a-popconfirm
            :title="$t('customRuleDeleteConfirm')"
            @confirm="removeRule(record.id)"
          >
            <a-button
              type="primary"
              size="small"
              danger
              ghost
              style="margin-left: 8px"
            >
              {{ $t('common.delete') }}
            </a-button>
          </a-popconfirm>
        </template>
      </template>
    </a-table>
  </a-card>

  <a-modal
    v-model:open="ruleOpen"
    :title="ruleForm.id ? $t('customRuleEdit') : $t('customRuleAdd')"
    @ok="saveRule"
  >
    <a-form layout="vertical">
      <a-form-item :label="$t('customRuleName')" required>
        <a-input v-model:value="ruleForm.name" />
      </a-form-item>
      <a-form-item :label="$t('customRulePattern')" required>
        <a-input
          v-model:value="ruleForm.pattern"
          :placeholder="'\\bdrop\\s+table\\b'"
        />
      </a-form-item>
      <a-form-item :label="$t('customRuleLevel')">
        <a-radio-group v-model:value="ruleForm.level">
          <a-radio value="error">{{ $t('customRuleLevelError') }}</a-radio>
          <a-radio value="warning">{{ $t('customRuleLevelWarn') }}</a-radio>
        </a-radio-group>
      </a-form-item>
      <a-form-item :label="$t('customRuleDesc')">
        <a-input v-model:value="ruleForm.desc" />
      </a-form-item>
      <a-form-item :label="$t('customRuleEnabled')">
        <a-switch v-model:checked="ruleEnabled" />
      </a-form-item>
    </a-form>
  </a-modal>

  <a-drawer
    v-model:open="is_open"
    force-render
    :title="$t('custom')"
    placement="right"
    :width="width"
  >
    <a-input
      v-model:value="desc"
      :placeholder="$t('common.table.name')"
    ></a-input>
    <br />
    <br />
    <rules
      ref="r"
      :desc="desc"
      :is-add="ist_add"
      @ok="
        turnState();
        get();
      "
    />
  </a-drawer>
</template>

<script setup lang="ts">
  import {
    CustomRuleItem,
    deleteCustomRule,
    deleteRules,
    getCustomRuleList,
    getCustomRulesList,
    saveCustomRule,
  } from '@/apis/rules';
  import { computed, onMounted, reactive, ref } from 'vue';
  import CommonMixins from '@/mixins/common';
  import rules from './rules.vue';
  import { useElementSize } from '@vueuse/core';
  import { useI18n } from 'vue-i18n';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();

  const { is_open, turnState } = CommonMixins();

  const card = ref();

  const ist_add = ref(false);

  const r = ref();

  const { width } = useElementSize(card);

  const desc = ref('');

  const { t } = useI18n();

  const col = [
    {
      title: t('common.table.name'),
      dataIndex: 'desc',
    },
    {
      // 引用数据源:改这条规则集前先看影响面(后端 SuperRolesList 聚合返回)
      title: t('ruleRefSources'),
      dataIndex: 'ref_sources',
    },
    {
      title: t('common.action'),
      dataIndex: 'action',
      slotName: 'action',
    },
  ];

  const list = ref([] as any);

  const onSearch = (vl: string) => {
    list.value = list.value.filter((item: any) => item.desc.indexOf(vl) !== -1);
  };

  const get = async () => {
    const { data } = await getCustomRulesList();
    list.value = data.payload;
  };

  // 被数据源引用的规则集后端会拒绝删除，并把数据源名字写在 text 里（拦截器已提示）
  const onDelete = async (id: number) => {
    const { data } = await deleteRules({ id });
    if (data.code !== 1200) return;
    message.success(t('ruleDeleteDone'));
    get();
  };

  // ---- 自定义审核规则（正则文本规则） ----
  const ruleList = ref<CustomRuleItem[]>([]);

  const ruleOpen = ref(false);

  const emptyRule = (): CustomRuleItem => ({
    name: '',
    desc: '',
    pattern: '',
    level: 'error',
    enabled: 1,
  });

  const ruleForm = reactive<CustomRuleItem>(emptyRule());

  const ruleEnabled = computed({
    get: () => ruleForm.enabled === 1,
    set: (v: boolean) => (ruleForm.enabled = v ? 1 : 0),
  });

  const ruleCol = [
    { title: t('customRuleName'), dataIndex: 'name', width: 180 },
    { title: t('customRulePattern'), dataIndex: 'pattern' },
    { title: t('customRuleLevel'), dataIndex: 'level', width: 100 },
    { title: t('customRuleEnabled'), dataIndex: 'enabled', width: 90 },
    { title: t('common.action'), dataIndex: 'action', width: 190 },
  ];

  const getRuleList = async () => {
    const { data } = await getCustomRuleList();
    ruleList.value = data.payload;
  };

  const openRule = (record?: CustomRuleItem) => {
    Object.assign(ruleForm, record ? { ...record } : { ...emptyRule(), id: undefined });
    ruleOpen.value = true;
  };

  const saveRule = async () => {
    const { data } = await saveCustomRule({ ...ruleForm });
    if (data.code !== 1200) return;
    message.success(t('customRuleSaved'));
    ruleOpen.value = false;
    getRuleList();
  };

  const toggleRule = async (record: CustomRuleItem, v: boolean) => {
    const { data } = await saveCustomRule({ ...record, enabled: v ? 1 : 0 });
    if (data.code !== 1200) return;
    record.enabled = v ? 1 : 0;
  };

  const removeRule = async (id?: number) => {
    const { data } = await deleteCustomRule({ id });
    if (data.code !== 1200) return;
    message.success(t('customRuleDeleted'));
    getRuleList();
  };

  onMounted(async () => {
    get();
    getRuleList();
  });
</script>

<style scoped></style>
