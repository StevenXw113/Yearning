<template>
  <a-row>
    <a-col :span="14">
      <a-input-search
        :placeholder="$t('ruleSearchTips')"
        enter-button
        style="max-width: 320px"
        @search="onSearch"
      />
    </a-col>
    <a-col :span="9" offset="1" style="text-align: right">
      <a-button style="margin-right: 8px" @click="helpOpen = true">{{
        $t('ruleHelp')
      }}</a-button>
      <a-button
        v-if="!isAdd"
        style="margin-right: 8px"
        @click="onHistory"
        >{{ $t('ruleHistory') }}</a-button
      >
      <a-button
        v-if="global"
        style="margin-right: 8px"
        @click="onCheckUpstream"
        >{{ $t('upstreamCheck') }}</a-button
      >
      <a-button type="primary" @click="onSave">{{
        $t('common.save')
      }}</a-button>
    </a-col>
  </a-row>
  <br />
  <a-table
    bordered
    :columns="col"
    :data-source="rules"
    :pagination="{
      pageSize: 20,
      showTotal: (total:number) => $t('common.count', { count: total }),
    }"
    size="small"
  >
    <template #bodyCell="{ column, record }">
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
          v-model:checked="engine[record.name]"
        ></a-switch>
        <a-input-number
          v-if="record.tp === 1"
          v-model:value="engine[record.name]"
        >
        </a-input-number>
        <a-input
          v-if="record.tp === 2"
          v-model:value="engine[record.name]"
        ></a-input>
        <a-textarea
          v-if="record.tp === 3"
          v-model:value="engine[record.name]"
          :rows="6"
        >
        </a-textarea>
      </template>
    </template>
  </a-table>

  <a-modal
    v-model:visible="helpOpen"
    :title="$t('ruleHelp')"
    :footer="null"
    :width="680"
  >
    <p>{{ $t('ruleHelpIntro') }}</p>
    <ul style="padding-left: 18px">
      <li>{{ $t('ruleHelpAdd') }}</li>
      <li>{{ $t('ruleHelpEnable') }}</li>
      <li>{{ $t('ruleHelpDelete') }}</li>
    </ul>
    <p style="margin-bottom: 0">{{ $t('ruleHelpDoc') }}</p>
  </a-modal>

  <a-drawer
    v-model:visible="historyOpen"
    :title="$t('ruleHistory')"
    placement="right"
    :width="560"
  >
    <a-table
      bordered
      size="small"
      :columns="historyCol"
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
  import { Rule, rule } from './rules';
  import { computed, ref } from 'vue';
  import {
    Rules,
    RuleHistory,
    updateRules,
    updateGlobalRules,
    addRules,
    checkUpstreamRules,
    getRuleHistory,
    rollbackRule,
  } from '@/apis/rules';
  import { message, Modal } from 'ant-design-vue';
  import { useI18n } from 'vue-i18n';

  const props = defineProps<{
    global?: boolean;
    desc: string;
    isAdd?: boolean;
  }>();

  const { t } = useI18n();

  const emit = defineEmits(['ok']);

  const col = [
    {
      title: t('common.table.name'),
      dataIndex: 'name',
      width: 50,
    },
    {
      title: t('common.table.type'),
      dataIndex: 'type',
      width: 80,
      filters: [
        { text: 'DDL', value: 'DDL' },
        { text: 'DML', value: 'DML' },
        { text: 'Online-DDL', value: 'Online-DDL' },
        { text: 'MongoDB', value: 'Mongo' },
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
      width: 300,
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

  const engine = ref({} as Rules);

  // 规则级别单独存一份（键为规则字段名），保存时合并进 audit_role.RuleLevel。
  const levels = ref({} as Record<string, string>);

  const rules = ref<Rule[]>(rule);

  const id = ref(0);

  const onSearch = (vl: string) => {
    rules.value = rule.filter((item) => item.desc.indexOf(vl) !== -1);
  };

  // 后端把级别放在 audit_role.RuleLevel 里，取回来单独渲染，保存时再合并回去。
  const applyRules = (r: Rules) => {
    engine.value = r;
    levels.value =
      ((r as unknown as { RuleLevel?: Record<string, string> }).RuleLevel ||
        {}) as Record<string, string>;
  };

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

  const onRules = (r: Rules, ids: number) => {
    applyRules(r);
    id.value = ids;
  };

  const onSave = async () => {
    const audit = {
      ...engine.value,
      RuleLevel: levels.value,
    } as unknown as Rules;
    if (props.isAdd) {
      await addRules({ desc: props.desc, audit_role: audit });
    } else {
      props.global
        ? await updateGlobalRules(audit)
        : await updateRules({
            desc: props.desc,
            audit_role: audit,
            id: id.value,
          });
    }
    emit('ok');
  };

  // 只检测：上游规则随引擎编译，页面不会自动更新任何文件。
  const onCheckUpstream = async () => {
    const { data } = await checkUpstreamRules();
    if (data.code !== 1200) return; // 非 1200 已由请求拦截器统一提示
    const { current, latest, hasUpdate } = data.payload;
    if (hasUpdate) {
      Modal.warning({
        title: t('upstreamNew'),
        content: t('upstreamNewDesc', { current, latest }),
      });
    } else {
      message.success(t('upstreamLatest', { version: current }));
    }
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
    { title: t('common.action'), dataIndex: 'action', width: 90 },
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
    applyRules(data.payload);
    message.success(t('ruleRollbackDone'));
    historyOpen.value = false;
  };

  defineExpose({
    onRules,
  });
</script>
