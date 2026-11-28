<template>
  <a-row :gutter="[16, 12]">
    <a-col :xs="24" :lg="12">
      <a-input-search
        :placeholder="$t('ruleSearchTips')"
        enter-button
        style="max-width: 320px"
        @search="onSearch"
      />
    </a-col>
    <a-col :xs="24" :lg="12" style="text-align: right">
      <a-space wrap>
        <a-button @click="helpOpen = true">{{ $t('ruleHelp') }}</a-button>
        <a-button v-if="!isAdd" @click="onHistory">{{
          $t('ruleHistory')
        }}</a-button>
        <a-button type="primary" @click="onSave">{{
          $t('common.save')
        }}</a-button>
      </a-space>
    </a-col>
  </a-row>
  <br />
  <a-segmented v-model:value="activeCat" :options="catOptions" />
  <br />
  <a-table
    bordered
    :columns="col"
    :scroll="{ x: 1100 }"
    :data-source="rules"
    :pagination="{
      pageSize: 20,
      showTotal: (total: number) => $t('common.count', { count: total }),
    }"
    size="small"
  >
    <template #bodyCell="{ column, record }">
      <template v-if="column.dataIndex === 'name'">
        <div>{{ record.name }}</div>
        <a-space v-if="record.ids || record.settingKey" wrap size="small">
          <a-tag v-for="id in record.ids || []" :key="id" color="blue">
            {{ id }}
          </a-tag>
          <a-tag v-if="record.settingKey" :key="record.settingKey" color="cyan">
            {{ record.settingKey }}
          </a-tag>
          <a-tag v-if="record.dbaApproval" :key="'dba'" color="red">DBA</a-tag>
        </a-space>
      </template>
      <template v-if="column.dataIndex === 'level'">
        <a-select
          size="small"
          style="width: 100%"
          :value="levels[record.name] || ''"
          :options="levelOptions(record)"
          @change="(v: string) => setLevel(record.name, v)"
        />
      </template>
      <template v-if="column.dataIndex === 'action'">
        <a-switch
          v-if="record.tp === 0"
          :checked="
            isDBAApprovalRule(record) ? true : Boolean(auditRole[record.name])
          "
          :disabled="isDBAApprovalRule(record)"
          @change="(v: boolean) => setEngineValue(record, v)"
        ></a-switch>
        <a-input-number
          v-if="record.tp === 1"
          v-model:value="auditRole[record.name]"
        >
        </a-input-number>
        <a-input
          v-if="record.tp === 2"
          v-model:value="auditRole[record.name]"
        ></a-input>
        <a-textarea
          v-if="record.tp === 3"
          v-model:value="auditRole[record.name]"
          :rows="6"
        >
        </a-textarea>
      </template>
    </template>
  </a-table>

  <a-modal
    v-model:open="helpOpen"
    :title="$t('ruleHelp')"
    :footer="null"
    :width="680"
  >
    <p>{{ $t('ruleHelpIntro') }}</p>
    <ul style="padding-left: 18px">
      <li>{{ $t('ruleHelpScope') }}</li>
      <li>{{ $t('ruleHelpAdd') }}</li>
      <li>{{ $t('ruleHelpEnable') }}</li>
      <li>{{ $t('ruleHelpLevel') }}</li>
      <li>{{ $t('ruleHelpBind') }}</li>
      <li>{{ $t('ruleHelpMongo') }}</li>
      <li>{{ $t('ruleHelpHistory') }}</li>
      <li>{{ $t('ruleHelpDelete') }}</li>
      <li>{{ $t('ruleHelpCustom') }}</li>
    </ul>
    <p style="margin-bottom: 0">{{ $t('ruleHelpSave') }}</p>
  </a-modal>

  <a-drawer
    v-model:open="historyOpen"
    :title="$t('ruleHistory')"
    placement="right"
    :width="560"
  >
    <a-table
      bordered
      size="small"
      :columns="historyCol"
      :scroll="{ x: 600 }"
      :data-source="history"
      :pagination="{ pageSize: 10 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'action'">
          <a-popconfirm
            :title="$t('ruleRollbackConfirm')"
            @confirm="onRollback(record.id)"
          >
            <a-button size="small" danger>{{ $t('ruleRollback') }}</a-button>
          </a-popconfirm>
        </template>
      </template>
    </a-table>
  </a-drawer>

  <a-back-top />
</template>

<script lang="ts" setup>
  import {
    Rule,
    rule,
    RULE_CATEGORIES,
    filterRules,
    filterByEngine,
    categoryCounts,
    type RuleCategory,
    type RuleEngine,
  } from './rules';
  import { computed, onMounted, ref } from 'vue';
  import {
    Rules,
    RuleHistory,
    getRulesList,
    updateRules,
    updateGlobalRules,
    addRules,
    getRuleHistory,
    rollbackRule,
    buildMongoRuleSetDefinition,
    updateMongoRuleSetDefinition,
    type RuleSetDefinition,
  } from '@/apis/rules';
  import { App } from 'ant-design-vue';
  const { message } = App.useApp();
  import { useI18n } from 'vue-i18n';

  const props = defineProps<{
    global?: boolean;
    desc: string;
    isAdd?: boolean;
    // 只显示该数据库类型适用的规则；不传 = 全部（开关仍是同一份全局配置）
    engine?: RuleEngine;
  }>();

  const { t } = useI18n();

  const emit = defineEmits(['ok']);

  const col = [
    {
      title: t('common.table.name'),
      dataIndex: 'name',
      width: 200,
    },
    {
      title: t('common.table.type'),
      dataIndex: 'type',
      width: 112,
      filters: [
        { text: 'DDL', value: 'DDL' },
        { text: 'DML', value: 'DML' },
        { text: 'Online-DDL', value: 'Online-DDL' },
        { text: 'MongoDB', value: 'Mongo' },
        { text: 'Redis', value: 'Redis' },
      ],
      onFilter: (value: string, record: any) => record.type.includes(value),
    },
    {
      title: t('common.desc'),
      dataIndex: 'desc',
    },
    {
      // 规则级别：拦截(错误) / 提示(警告) / 观察。后两者不拦提交，用于新规则灰度上线。
      title: t('ruleLevelTitle'),
      dataIndex: 'level',
      width: 130,
    },
    {
      title: t('common.action'),
      dataIndex: 'action',
      width: 240,
    },
  ];

  // 未配置时显示「默认(拦截)」这种直白写法：括号里是这条规则自带的默认级别
  // （rules.ts 的 level，由 archguard 核对与引擎 DefaultLevel 一致；SQL 规则缺省一律 error）。
  const shortLevel = (v?: string) => {
    switch (v) {
      case 'warn':
        return t('ruleLevelShortWarn');
      case 'observe':
        return t('ruleLevelShortObserve');
      default:
        return t('ruleLevelShortError');
    }
  };

  const levelOptions = (r: Rule) => [
    { value: '', label: `${t('ruleLevelDefault')}(${shortLevel(r.level)})` },
    { value: 'error', label: t('ruleLevelError') },
    { value: 'warn', label: t('ruleLevelWarn') },
    { value: 'observe', label: t('ruleLevelObserve') },
  ];

  const auditRole = ref({} as Rules);
  const currentDefinition = ref<RuleSetDefinition | null>(null);

  const isMongoRuleSet = computed(() => props.engine === 'mongo');

  const isDBAApprovalRule = (r: Rule) => Boolean(r.dbaApproval);

  const setEngineValue = (r: Rule, v: boolean) => {
    if (isDBAApprovalRule(r)) return;
    auditRole.value[r.name] = v;
  };

  // 规则级别单独存一份（键为规则字段名），保存时合并进 audit_role.RuleLevel。
  const levels = ref({} as Record<string, string>);

  // 列表 = 分类页签 × 搜索关键字 的交集;rule 是静态名单,过滤全部走计算属性
  const activeCat = ref<RuleCategory | 'all'>('all');
  const keyword = ref('');
  // 按数据库类型过滤后的规则池：分类页签的计数也要基于它，否则数字与实际行数对不上
  const pool = computed(() => filterByEngine(rule, props.engine));
  const counts = computed(() => categoryCounts(pool.value));
  const catOptions = computed(() => [
    {
      label: `${t('ruleCatAll')} (${counts.value.all})`,
      value: 'all' as const,
    },
    ...RULE_CATEGORIES.map((c) => ({
      label: `${t(c.i18n)} (${counts.value[c.key]})`,
      value: c.key,
    })),
  ]);
  const rules = computed(() =>
    filterRules(pool.value, activeCat.value).filter(
      (item) => keyword.value === '' || item.desc.indexOf(keyword.value) !== -1,
    ),
  );

  const id = ref(0);

  const onSearch = (vl: string) => {
    keyword.value = vl;
  };

  // 后端把级别放在 audit_role.RuleLevel 里，取回来单独渲染，保存时再合并回去。
  const applyRules = (r: Rules) => {
    auditRole.value = { ...r };
    if (isMongoRuleSet.value) {
      // DBA 硬底线规则在 UI 与 v2 Definition 中都不可关闭。
      auditRole.value.MongoForbidDropCollection = true;
      auditRole.value.MongoForbidAdminCommand = true;
    }
    levels.value = ((r as unknown as { RuleLevel?: Record<string, string> })
      .RuleLevel || {}) as Record<string, string>;
  };

  // 全局规则自己拉数据（类型页签切换会重建组件，父级注入会丢）；规则集仍由父级 onRules 注入
  onMounted(async () => {
    if (!props.global) return;
    const { data } = await getRulesList();
    applyRules(data.payload as unknown as Rules);
  });

  // 级别下拉：未配置就是「默认」——引擎按规则自带的默认级别处理
  // （安全类 error、结构与性能类 warn）。这里必须把键删掉而不是写空串，
  // 否则页面会把「没人配过」显示成「配了某个值」。
  const setLevel = (name: string, v: string) => {
    if (v === '') {
      delete levels.value[name];
      return;
    }
    levels.value[name] = v;
  };

  const onRules = (
    r: Rules,
    ids: number,
    definition?: RuleSetDefinition | null,
  ) => {
    applyRules(r);
    id.value = ids;
    // v2 Definition 是引擎事实来源；AuditRole 仅作为旧页面状态兼容层。
    void definition;
  };

  const onSave = async () => {
    const audit = {
      ...auditRole.value,
      RuleLevel: levels.value,
    } as unknown as Rules;
    if (props.isAdd) {
      await addRules({
        desc: props.desc,
        audit_role: audit,
        ...(isMongoRuleSet.value
          ? {
              definition: buildMongoRuleSetDefinition(audit),
              dialect: 'mongodb',
              scope: 'data_source',
            }
          : {}),
      });
    } else if (props.global) {
      await updateGlobalRules(audit);
    } else if (isMongoRuleSet.value) {
      await updateRules({
        desc: props.desc,
        audit_role: audit,
        definition: updateMongoRuleSetDefinition(
          currentDefinition.value,
          audit,
        ),
        dialect: 'mongodb',
        scope: 'data_source',
        id: id.value,
      });
    } else {
      await updateRules({
        desc: props.desc,
        audit_role: audit,
        id: id.value,
      });
    }
    emit('ok');
  };

  // 全局规则的历史挂在 rule_id=0 上，规则集历史挂在各自的 id 上（与后端一致）。
  const ruleId = computed(() => (props.global ? 0 : id.value));

  const historyOpen = ref(false);

  const helpOpen = ref(false);

  const history = ref<RuleHistory[]>([]);

  const historyCol = [
    { title: t('ruleCreatedAt'), dataIndex: 'created_at', width: 150 },
    { title: t('ruleOperator'), dataIndex: 'operator', width: 110 },
    { title: t('common.desc'), dataIndex: 'note' },
    {
      title: t('common.action'),
      dataIndex: 'action',
      width: 112,
      className: 'table-action-cell',
    },
  ];

  const onHistory = async () => {
    const { data } = await getRuleHistory(ruleId.value);
    if (data.code !== 1200) return;
    history.value = data.payload;
    historyOpen.value = true;
  };

  const onRollback = async (hid: number) => {
    const { data } = await rollbackRule(hid);
    if (data.code !== 1200) return;
    currentDefinition.value = data.payload.definition || null;
    applyRules(data.payload.audit_role);
    message.success(t('ruleRollbackDone'));
    historyOpen.value = false;
  };

  defineExpose({
    onRules,
  });
</script>
