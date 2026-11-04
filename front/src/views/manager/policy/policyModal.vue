<template>
  <a-modal
    v-model:open="is_open"
    :title="props.title"
    :width="800"
    @ok="postPolicy"
  >
    <a-form layout="vertical">
      <a-form-item :label="$t('common.policy.group') + $t('common.table.name')">
        <a-input v-model:value="selfRuse.name" :disabled="isEdit"></a-input>
      </a-form-item>
      <template v-for="i in ranges" :key="i">
        <a-transfer
          v-model:target-keys="selfRuse[`${i}_source`]"
          :titles="[
            ' ' +
              $t('common.all') +
              `${i.toLocaleUpperCase()} ` +
              $t('common.policy'),
            ' ' +
              $t('common.selected') +
              `${i.toLocaleUpperCase()} ` +
              $t('common.policy'),
          ]"
          :data-source="optionsOf(i)"
          show-search
          :render="(item: any) => item.title"
          :list-style="{
            width: '400px',
            height: '300px',
          }"
        />
        <a-divider></a-divider>
      </template>
    </a-form>
  </a-modal>
</template>

<script lang="ts" setup>
  import {
    getPolicySources,
    Policy,
    PolicyPost,
    PolicyRuse,
    updatePolicy,
  } from '@/apis/policy';
  import CommonMixins from '@/mixins/common';
  import { computed, onMounted, reactive, unref, ref } from 'vue';

  const emit = defineEmits(['success']);

  const { is_open, turnState } = CommonMixins();

  const props = defineProps<{
    title: string;
  }>();

  const isEdit = ref(false);

  const ruse = reactive<PolicyRuse>({
    source: [],
    query: [],
  });

  const selfRuse = reactive<PolicyPost>({
    ddl_source: [],
    dml_source: [],
    query_source: [],
    name: '',
    id: 0,
  });

  let initRune = Object.assign({}, selfRuse);

  // 三个权限维度：ddl/dml 走数据源列表，query 走查询数据源列表
  const ranges = ['ddl', 'dml', 'query'] as const;

  // antd-vue 4 的 Transfer 按 key/title 渲染列表项，必须显式映射，否则是空白行。
  // 用 computed 缓存：若每次渲染都新建数组，Transfer 会认为 dataSource 变了并重算，
  // 表现为列表闪烁、已选项回弹到左侧。
  const toOptions = (list: { source?: string; source_id?: string }[]) =>
    list.map((item) => ({
      key: item.source_id as string,
      title: item.source ?? '',
    }));

  const sourceOptions = computed(() => toOptions(ruse.source));

  const queryOptions = computed(() => toOptions(ruse.query));

  const optionsOf = (type: (typeof ranges)[number]) =>
    type === 'query' ? queryOptions.value : sourceOptions.value;

  const postPolicy = async () => {
    await updatePolicy(selfRuse);
    turnState();
    emit('success');
  };

  const editPolicy = (vl: Policy) => {
    isEdit.value = true;
    selfRuse.id = vl.id;
    selfRuse.name = vl.name;
    const p = vl.permissions ?? ({} as PolicyPost);
    selfRuse.query_source = p.query_source ?? [];
    selfRuse.dml_source = p.dml_source ?? [];
    selfRuse.ddl_source = p.ddl_source ?? [];
    turnState();
  };

  const newPolicy = () => {
    isEdit.value = false;
    Object.assign(selfRuse, unref(initRune));
    turnState();
  };

  onMounted(async () => {
    const { data } = await getPolicySources();
    ruse.source = data.payload.source;
    ruse.query = data.payload.query;
  });

  defineExpose({
    newPolicy,
    editPolicy,
  });
</script>
