<template>
  <PageHeader
    :title="$t('menu.manage.source')"
    :sub-title="$t('common.db.sub')"
  />
  <div class="datasource-toolbar">
    <a-button type="primary" @click="createOpen = true">
      <template #icon><PlusOutlined /></template>
      {{ $t('common.new') }} {{ $t('menu.manage.source') }}
    </a-button>
  </div>
  <section class="datasource-list" :aria-label="$t('menu.manage.source')">
    <DBTable ref="dbTable" />
  </section>
  <a-drawer
    v-model:open="createOpen"
    :title="$t('common.new') + ' ' + $t('menu.manage.source')"
    width="min(480px, 100vw)"
    :mask-closable="false"
    destroy-on-close
  >
    <DBForm @created="sourceCreated" />
  </a-drawer>
</template>

<script lang="ts" setup>
  import { PlusOutlined } from '@ant-design/icons-vue';
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import { updateUserPrincipal } from '@/apis/user';
  import { onMounted, ref } from 'vue';
  import DBForm from './dbForm.vue';
  import DBTable from './dbTable.vue';
  import { useCommonStore } from '@/store';
  import { queryIDCList } from '@/apis/source';

  const dbTable = ref<InstanceType<typeof DBTable>>();

  const createOpen = ref(false);
  const sourceCreated = () => {
    createOpen.value = false;
    dbTable.value?.refresh();
  };

  const commonStore = useCommonStore();

  const getPrincipal = async () => {
    const { data } = await updateUserPrincipal();
    commonStore.DB_SET_PRINCIPAL(data.payload);
  };

  onMounted(async () => {
    const { data } = await queryIDCList();
    commonStore.GET_IDC(data.payload);
    commonStore.GET_FLOWS();
    getPrincipal();
  });
</script>

<style scoped>
  .datasource-toolbar {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 16px;
  }
  .datasource-list {
    min-width: 0;
  }
</style>
