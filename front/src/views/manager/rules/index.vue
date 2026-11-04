<template>
  <a-page-header
    :title="$t('common.rule')"
    :sub-title="$t('common.rule.sub')"
    :ghost="false"
  >
    <template #footer>
      <!-- 各类型页签读写的是同一份全局规则，只是过滤出该数据库适用的规则；
           切走会销毁重建，重新拉取，避免多页签之间互相覆盖 -->
      <a-tabs v-model:activeKey="active" destroy-inactive-tab-pane>
        <a-tab-pane key="global" :tab="$t('global')">
          <br />
          <rules global desc="" />
        </a-tab-pane>
        <a-tab-pane v-for="e in RULE_ENGINES" :key="e.key" :tab="e.label">
          <br />
          <rules global desc="" :engine="e.key" />
        </a-tab-pane>
        <a-tab-pane key="refer" :tab="$t('custom_list')">
          <custom />
        </a-tab-pane>
      </a-tabs>
    </template>
  </a-page-header>
</template>

<script setup lang="ts">
  import rules from './rules.vue';
  import custom from './custom.vue';
  import { RULE_ENGINES } from './rules';
  import { ref } from 'vue';

  const active = ref('global');
</script>

<style lang="scss" scoped></style>
