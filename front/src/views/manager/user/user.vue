<template>
  <PageHeader :title="title.title" :sub-title="title.subTitle"></PageHeader>
  <a-row :gutter="[16, 16]">
    <a-col :xs="24" :sm="6">
      <a-card :title="$t('user.form.title')">
        <!-- 注册成功后由本组件中继给右栏用户表刷新（替代原全局 EventBus） -->
        <RegisterForm is-manager @registered="userTable?.refresh()"></RegisterForm>
      </a-card>
    </a-col>
    <a-col :xs="24" :sm="18">
      <UserTable ref="userTable"></UserTable>
    </a-col>
  </a-row>
</template>

<script lang="ts" setup>
  import PageHeader from '@/components/pageHeader/pageHeader.vue';
  import RegisterForm from '@/components/user/registerForm.vue';
  import UserTable from '@/views/manager/user/userTable.vue';
  import { ref } from 'vue';
  import { useI18n } from 'vue-i18n';

  const { t } = useI18n();

  const userTable = ref<InstanceType<typeof UserTable>>();

  const title = {
    title: t('user.title'),
    subTitle: t('user.desc'),
  };
</script>

<style scoped></style>
