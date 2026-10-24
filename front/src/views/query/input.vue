<template>
  <a-form layout="inline">
    <a-form-item :label="$t('common.table.source')">
      <span>{{ commonStore.queryInfo.source }}</span>
    </a-form-item>
    <a-form-item :label="$t('common.choose') + $t('common.table.schema')">
      <a-select
        v-model:value="schema"
        style="width: 200px"
        show-search
        :dropdown-match-select-width="false"
        @change="selected"
      >
        <a-select-option
          v-for="i in commonStore.schemaList"
          :key="i"
          :value="i"
          >{{ i }}</a-select-option
        >
      </a-select>
    </a-form-item>
  </a-form>
  <div style="height: 5px;"></div>
  <Editor
    ref="query_editor"
    :container-id="props.id"
    is-query
    @get-values="getValues"
  ></Editor>
  <br />
  <Table :id="id" ref="tbl" :height="300"></Table>
</template>

<script lang="ts" setup>
  import Editor from '@/components/editor/editor.vue';
  import Table from './table.vue';
  import { useOrderStore, useCommonStore } from '@/store';
  import { computed, ref } from 'vue';

  const emit = defineEmits(['onChange']);

  const props = defineProps<{
    id: string;
  }>();

  const query_editor = ref();

    const orderStore = useOrderStore();
  const commonStore = useCommonStore();

  const tbl = ref();

  const schema = computed({
    get() {
      return commonStore.schema;
    },
    set(v: string) {
      commonStore.SET_SCHEMA(v);
    },
  });

  const selected = () => {
    emit('onChange');
  };

  const getValues = (vl: string) => {
    orderStore.SET_QUERY_HISTORY(vl);
    tbl.value.runResults(schema.value, vl);
  };
</script>
