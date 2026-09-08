<template>
  <a-button type="primary" block @click="updateSettingInfo(config)">{{
    $t('common.save')
  }}</a-button>
</template>

<script setup lang="ts">
  import { updateSettingInfo, Settings } from '@/apis/setting';
  import { inject } from 'vue';
  const config = inject('config') as Settings;
</script>

<style scoped></style>
