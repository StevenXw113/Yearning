<template>
  <a-menu v-model:selectedKeys="selectedKey" mode="inline" @click="changeMenu">
    <a-menu-item key="/home">
      <HomeOutlined />
      <span>{{ $t('menu.home') }}</span>
    </a-menu-item>
    <a-menu-item key="/advisor">
      <DotChartOutlined />
      <span>{{ $t('menu.order.advisor') }}</span>
    </a-menu-item>
    <a-menu-item key="/server/order/common/list">
      <UserOutlined />
      <span>{{ $t('menu.order.self') }}</span>
    </a-menu-item>
    <a-menu-item key="/apply">
      <ConsoleSqlOutlined />
      <span>{{ $t('menu.order.apply') }}</span>
    </a-menu-item>
    <a-menu-item
      v-if="userStore.account.is_record === 1"
      key="/comptroller/order/record"
    >
      <AreaChartOutlined />
      <span>{{ $t('menu.comptroller.title') }}</span>
    </a-menu-item>
    <a-sub-menu :title="$t('menu.order.title')">
      <template #icon>
        <AuditOutlined />
      </template>
      <a-menu-item key="/server/order/audit/list">
        <span>{{ $t('menu.order.order') }}</span>
      </a-menu-item>
      <a-menu-item key="/server/query/list">
        <span>{{ $t('menu.order.query') }}</span>
      </a-menu-item>
    </a-sub-menu>
    <a-sub-menu
      v-if="userStore.account.user === 'admin'"
      :title="$t('menu.manage')"
    >
      <template #icon>
        <CloudSyncOutlined />
      </template>
      <a-menu-item key="/manager/user">
        <UserAddOutlined />
        <span>{{ $t('menu.manage.user') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/db">
        <CloudServerOutlined />
        <span>{{ $t('menu.manage.source') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/flow">
        <PartitionOutlined />
        <span>{{ $t('menu.manage.flow') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/policy">
        <UsergroupAddOutlined />
        <span>{{ $t('menu.manage.policy') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/rules">
        <CrownOutlined />
        <span>{{ $t('menu.manage.rule') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/autotask">
        <PaperClipOutlined />
        <span>{{ $t('menu.manage.auto') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/board">
        <DotChartOutlined />
        <span>{{ $t('menu.manage.board') }}</span>
      </a-menu-item>
      <a-menu-item key="/manager/setting">
        <ToolOutlined />
        <span>{{ $t('menu.manage.setting') }}</span>
      </a-menu-item>
    </a-sub-menu>
  </a-menu>
</template>

<script lang="ts" setup>
  import {
    AreaChartOutlined,
    DotChartOutlined,
    HomeOutlined,
    AuditOutlined,
    ToolOutlined,
    PaperClipOutlined,
    CloudServerOutlined,
    CloudSyncOutlined,
    UsergroupAddOutlined,
    UserAddOutlined,
    ConsoleSqlOutlined,
    PartitionOutlined,
    CrownOutlined,
    UserOutlined,
  } from '@ant-design/icons-vue';
  import { useUserStore, useMenuStore } from '@/store';
  import { ref } from 'vue';
  import router from '@/router';

  const emit = defineEmits(['close']);

    const userStore = useUserStore();
  const menuStore = useMenuStore();

  const selectedKey = ref(menuStore.selectedKey);

  const changeMenu = (vl: { keyPath: string[]; key: string }) => {
    menuStore.CHANGE_SELECTED(vl.keyPath);
    router.push(vl.key).finally(() => emit('close'));
  };
</script>
