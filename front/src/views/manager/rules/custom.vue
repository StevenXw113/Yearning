<template>
  <a-card ref="card">
    <a-row>
      <a-col :span="15">
        <a-input-search
          :placeholder="$t('ruleSearchTips')"
          enter-button
          style="max-width: 320px"
          @search="onSearch"
        />
      </a-col>
      <a-col :span="9" style="text-align: right">
        <a-space>
          <a-button @click="openManifest">
            {{ $t('ruleManifest') }}
          </a-button>
          <a-button @click="openImport">{{ $t('ruleSetImport') }}</a-button>
          <a-button type="primary" @click="openCreate">
            {{ $t('common.add') }}
          </a-button>
        </a-space>
      </a-col>
    </a-row>
    <br />
    <a-table bordered :columns="col" :data-source="list">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'ref_sources'">
          <a-tag v-for="s in record.ref_sources || []" :key="s">{{ s }}</a-tag>
          <span v-if="!record.ref_count">{{ $t('ruleRefNone') }}</span>
        </template>
        <template v-if="column.dataIndex === 'dialect'">
          <a-tag :color="record.dialect ? 'blue' : 'default'">
            {{ record.dialect || 'legacy' }}
          </a-tag>
        </template>
        <template v-if="column.dataIndex === 'rule_ids'">
          <a-space wrap size="small">
            <a-tag
              v-for="id in ruleSetStableIds(record)"
              :key="id"
              color="geekblue"
            >
              {{ id }}
            </a-tag>
            <span v-if="!ruleSetStableIds(record).length">—</span>
          </a-space>
        </template>
        <template v-if="column.dataIndex === 'action'">
          <a-space size="small">
            <a-button size="small" @click="openProfile(record)">
              {{ $t('common.profile') }}
            </a-button>
            <a-button size="small" @click="onExport(record)">
              {{ $t('ruleSetExport') }}
            </a-button>
            <a-popconfirm
              :title="$t('ruleDeleteConfirm')"
              @confirm="onDelete(record.id)"
            >
              <a-button type="primary" size="small" danger ghost>
                {{ $t('common.delete') }}
              </a-button>
            </a-popconfirm>
          </a-space>
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
    <a-form layout="vertical">
      <a-form-item :label="$t('common.table.name')" required>
        <a-input v-model:value="desc" />
      </a-form-item>
      <a-form-item :label="$t('ruleSetType')">
        <a-radio-group
          v-model:value="ruleSetType"
          :disabled="!ist_add && !!currentRecord"
          :options="ruleSetTypeOptions"
        />
        <div style="color: #8c8c8c; font-size: 12px; margin-top: 4px">
          {{ $t('ruleSetTypeTips') }}
        </div>
      </a-form-item>
    </a-form>
    <rules
      ref="r"
      :desc="desc"
      :is-add="ist_add"
      :engine="ruleSetType === 'mongo' ? 'mongo' : undefined"
      @ok="
        turnState();
        get();
      "
    />
  </a-drawer>

  <a-modal
    v-model:open="importOpen"
    :title="$t('ruleSetImport')"
    :confirm-loading="importLoading"
    @ok="saveImport"
  >
    <a-form layout="vertical">
      <a-form-item :label="$t('common.table.name')" required>
        <a-input v-model:value="importDesc" />
      </a-form-item>
      <a-form-item
        v-if="isSqlRuleSetImport"
        :label="$t('ruleSetTargetDialect')"
        :extra="$t('ruleSetTargetDialectTips')"
        required
      >
        <a-select
          v-model:value="importTargetDialect"
          :options="importTargetDialectOptions"
        />
      </a-form-item>
      <a-form-item :label="$t('ruleSetFile')" required>
        <a-upload
          accept=".json"
          :before-upload="onImportFile"
          :max-count="1"
          :show-upload-list="false"
        >
          <a-button>
            {{ importFileName || $t('ruleSetSelectFile') }}
          </a-button>
        </a-upload>
      </a-form-item>
      <a-alert type="info" show-icon :message="$t('ruleSetImportTips')" />
    </a-form>
  </a-modal>

  <a-modal
    v-model:open="manifestOpen"
    :title="$t('ruleManifest')"
    width="920px"
    :footer="null"
  >
    <a-table
      bordered
      size="small"
      :columns="manifestCol"
      :data-source="manifest"
      :pagination="{ pageSize: 10 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'id'">
          <code>{{ record.id }}</code>
        </template>
        <template v-if="column.dataIndex === 'requires_dba_approval'">
          <a-tag v-if="record.requires_dba_approval" color="red">DBA</a-tag>
          <span v-else>—</span>
        </template>
        <template v-if="column.dataIndex === 'param_type'">
          {{
            record.param_type === 'none'
              ? '—'
              : `${record.param_type} ${record.param_key || ''}`
          }}
        </template>
      </template>
    </a-table>
  </a-modal>
</template>

<script setup lang="ts">
  import {
    CustomRule,
    CustomRuleItem,
    RuleManifest,
    RuleManifestRule,
    RuleSetImportPayload,
    deleteCustomRule,
    deleteRules,
    exportRuleSet,
    getCustomRuleList,
    getCustomRulesList,
    getRulesManifest,
    importRuleSet,
    ruleSetStableIds,
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

  const currentRecord = ref<CustomRule | null>(null);

  const ruleSetType = ref<'legacy' | 'mongo'>('legacy');

  const { t } = useI18n();

  const ruleSetTypeOptions = [
    { label: t('ruleSetTypeLegacy'), value: 'legacy' },
    { label: 'MongoDB v2', value: 'mongo' },
  ];

  const col = [
    { title: t('common.table.name'), dataIndex: 'desc', width: 180 },
    { title: t('ruleSetDialect'), dataIndex: 'dialect', width: 100 },
    { title: t('ruleSetScope'), dataIndex: 'scope', width: 120 },
    { title: t('ruleStableIds'), dataIndex: 'rule_ids' },
    {
      // 引用数据源:改这条规则集前先看影响面(后端 SuperRolesList 聚合返回)
      title: t('ruleRefSources'),
      dataIndex: 'ref_sources',
      width: 180,
    },
    {
      title: t('common.action'),
      dataIndex: 'action',
      width: 210,
    },
  ];

  const list = ref<CustomRule[]>([]);

  const onSearch = (vl: string) => {
    list.value = list.value.filter((item) => item.desc.indexOf(vl) !== -1);
  };

  const get = async () => {
    const { data } = await getCustomRulesList();
    list.value = data.payload;
  };

  const isMongoRuleSet = (record: CustomRule) =>
    record.dialect === 'mongodb' || record.definition?.dialect === 'mongodb';

  const openCreate = () => {
    ist_add.value = true;
    currentRecord.value = null;
    ruleSetType.value = 'legacy';
    desc.value = '';
    is_open.value = true;
  };

  const openProfile = (record: CustomRule) => {
    ist_add.value = false;
    currentRecord.value = record;
    ruleSetType.value = isMongoRuleSet(record) ? 'mongo' : 'legacy';
    desc.value = record.desc;
    r.value?.onRules(record.audit_role, record.id || 0, record.definition);
    is_open.value = true;
  };

  // 被数据源引用的规则集后端会拒绝删除，并把数据源名字写在 text 里（拦截器已提示）
  const onDelete = async (id?: number) => {
    const { data } = await deleteRules({ id });
    if (data.code !== 1200) return;
    message.success(t('ruleDeleteDone'));
    get();
  };

  const onExport = async (record: CustomRule) => {
    if (!record.id) return;
    const { data } = await exportRuleSet(record.id);
    if (data.code !== 1200) return;
    const blob = new Blob([JSON.stringify(data.payload, null, 2)], {
      type: 'application/json',
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${record.desc || `rule-set-${record.id}`}.ruleset.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  // ---- 规则集导入 ----
  const importOpen = ref(false);
  const importDesc = ref('');
  const importFileName = ref('');
  const importPayload = ref<RuleSetImportPayload | null>(null);
  const importTargetDialect = ref<'mysql' | 'postgres'>('mysql');
  const importLoading = ref(false);

  const importTargetDialectOptions = [
    { label: 'MySQL', value: 'mysql' },
    { label: 'PostgreSQL', value: 'postgres' },
  ];

  const isSqlRuleSetImport = computed(() => {
    const payload = importPayload.value;
    return Boolean(
      payload &&
        'export_version' in payload &&
        payload.export_version === 1 &&
        payload.dialect === 'sql',
    );
  });

  const openImport = () => {
    importDesc.value = '';
    importPayload.value = null;
    importFileName.value = '';
    importTargetDialect.value = 'mysql';
    importOpen.value = true;
  };

  const onImportFile = async (file: File) => {
    try {
      importPayload.value = JSON.parse(await file.text());
      importFileName.value = file.name;
    } catch {
      importPayload.value = null;
      importFileName.value = '';
      message.error(t('ruleSetInvalidJson'));
    }
    return false;
  };

  const saveImport = async () => {
    if (!importDesc.value.trim() || !importPayload.value) {
      message.warning(t('ruleSetImportRequired'));
      return;
    }
    importLoading.value = true;
    const { data } = await importRuleSet(
      importDesc.value.trim(),
      importPayload.value,
      importTargetDialect.value,
    );
    importLoading.value = false;
    if (data.code !== 1200) return;
    message.success(t('ruleSetImportDone'));
    importOpen.value = false;
    get();
  };

  // ---- 规则清单 ----
  const manifestOpen = ref(false);
  const manifest = ref<RuleManifestRule[]>([]);

  const manifestCol = [
    { title: t('ruleStableId'), dataIndex: 'id', width: 230 },
    { title: t('common.table.name'), dataIndex: 'name', width: 160 },
    { title: t('common.desc'), dataIndex: 'desc' },
    { title: t('ruleLegacyId'), dataIndex: 'legacy_id', width: 220 },
    { title: t('ruleLevelTitle'), dataIndex: 'default_level', width: 100 },
    { title: t('ruleParam'), dataIndex: 'param_type', width: 150 },
    {
      title: 'DBA',
      dataIndex: 'requires_dba_approval',
      width: 90,
    },
  ];

  const openManifest = async () => {
    manifestOpen.value = true;
    const { data } = await getRulesManifest();
    if (data.code !== 1200) return;
    manifest.value = (data.payload as RuleManifest).rules;
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
    Object.assign(
      ruleForm,
      record ? { ...record } : { ...emptyRule(), id: undefined },
    );
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
