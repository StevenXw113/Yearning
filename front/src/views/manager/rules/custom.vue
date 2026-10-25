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
            <a-button size="small" danger ghost style="margin-left: 8px">{{
              $t('common.delete')
            }}</a-button>
          </a-popconfirm>
        </template>
      </template>
    </a-table>
  </a-card>

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
  import { deleteRules, getCustomRulesList } from '@/apis/rules';
  import { onMounted, ref } from 'vue';
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

  onMounted(async () => {
    get();
  });
</script>

<style scoped></style>
