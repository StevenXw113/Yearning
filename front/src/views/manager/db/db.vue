<template>
  <PageHeader
    :title="$t('menu.manage.source')"
    :sub-title="$t('common.db.sub')"
  ></PageHeader>
  <a-row :gutter="[16, 16]">
    <a-col :xs="24" :sm="6">
      <a-card :title="$t('common.new') + ' ' + $t('menu.manage.source')">
        <!-- 新建成功后由本组件中继给右栏列表刷新（替代原全局 EventBus） -->
        <DBForm @created="dbTable?.refresh()"></DBForm>
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="18">
      <DBTable ref="dbTable"></DBTable>
    </a-col>
  </a-row>
</template>

<script lang="ts" setup>
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import { updateUserPrincipal } from '@/apis/user';
  import { onMounted, ref } from 'vue';
  import DBForm from './dbForm.vue';
  import DBTable from './dbTable.vue';
  import { useCommonStore } from '@/store';
  import { queryIDCList } from '@/apis/source';

  const dbTable = ref<InstanceType<typeof DBTable>>();

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

<style scoped></style>
